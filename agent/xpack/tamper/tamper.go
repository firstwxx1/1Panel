package tamper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Web tamper protection: a baseline (hash of every file) is taken for each protected site and
// scanned against periodically. Changes are logged and, optionally, undone from a private copy.

const (
	maxBackupSize = 10 << 20 // bigger files are hashed but not copied, so they cannot be restored
	maxEvents     = 2000
)

type SiteCfg struct {
	Enabled    bool      `json:"enabled"`
	Restore    bool      `json:"restore"`
	Excludes   []string  `json:"excludes"`
	BaselineAt time.Time `json:"baselineAt"`
	Files      int       `json:"files"`
}

type Config struct {
	Enabled  bool               `json:"enabled"`
	Interval int                `json:"interval"` // seconds between scans
	Sites    map[string]SiteCfg `json:"sites"`
}

type Event struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Site   string    `json:"site"`
	Kind   string    `json:"kind"`   // modified | added | deleted
	Path   string    `json:"path"`   // relative to the site root
	Action string    `json:"action"` // none | restored | quarantined | restore_failed
	Hash   string    `json:"hash"`
}

type entry struct {
	Hash string      `json:"h"`
	Size int64       `json:"s"`
	MT   int64       `json:"t"`
	Mode os.FileMode `json:"m"`
	UID  int         `json:"u"`
	GID  int         `json:"g"`
}

func owner(info fs.FileInfo) (int, int) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}

type Store struct {
	dir string
	mu  sync.Mutex
	cfg Config
	evs []Event
	// "site|kind|path|hash" already reported, so detect-only mode does not repeat itself
	seen map[string]bool
	base map[string]map[string]entry
	next int64
}

var (
	storeMu sync.Mutex
	stores  = map[string]*Store{}
)

func Open(dir string) (*Store, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if s, ok := stores[dir]; ok {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, cfg: Config{Interval: 60, Sites: map[string]SiteCfg{}}, seen: map[string]bool{}, base: map[string]map[string]entry{}}
	if b, err := os.ReadFile(path.Join(dir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &s.cfg)
	}
	if s.cfg.Sites == nil {
		s.cfg.Sites = map[string]SiteCfg{}
	}
	if s.cfg.Interval < 10 {
		s.cfg.Interval = 60
	}
	if b, err := os.ReadFile(path.Join(dir, "events.json")); err == nil {
		_ = json.Unmarshal(b, &s.evs)
	}
	for _, e := range s.evs {
		s.seen[seenKey(e)] = true
		if e.ID > s.next {
			s.next = e.ID
		}
	}
	stores[dir] = s
	return s, nil
}

func seenKey(e Event) string { return e.Site + "|" + e.Kind + "|" + e.Path + "|" + e.Hash }

func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Sites = map[string]SiteCfg{}
	for k, v := range s.cfg.Sites {
		c.Sites[k] = v
	}
	return c
}

func (s *Store) saveCfgLocked() error {
	b, _ := json.MarshalIndent(s.cfg, "", "  ")
	return os.WriteFile(path.Join(s.dir, "config.json"), b, 0o600)
}

func (s *Store) SetGlobal(enabled bool, interval int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if interval < 10 {
		interval = 10
	}
	s.cfg.Enabled, s.cfg.Interval = enabled, interval
	return s.saveCfgLocked()
}

// SetSite updates one site's switches; the caller rebuilds the baseline when it is missing.
func (s *Store) SetSite(alias string, enabled, restore bool, excludes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg.Sites[alias]
	c.Enabled, c.Restore = enabled, restore
	var clean []string
	for _, e := range excludes {
		if e = strings.TrimSpace(e); e != "" {
			clean = append(clean, e)
		}
	}
	c.Excludes = clean
	s.cfg.Sites[alias] = c
	return s.saveCfgLocked()
}

func (s *Store) Events(site string, page, size int) ([]Event, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []Event
	for i := len(s.evs) - 1; i >= 0; i-- { // newest first
		if site == "" || s.evs[i].Site == site {
			all = append(all, s.evs[i])
		}
	}
	total := len(all)
	if size <= 0 {
		size = 20
	}
	if page < 1 {
		page = 1
	}
	from := (page - 1) * size
	if from > total {
		from = total
	}
	to := from + size
	if to > total {
		to = total
	}
	return all[from:to], total
}

// Since returns events newer than the given id, plus the latest id (for alert pollers).
func (s *Store) Since(id int64) ([]Event, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Event
	for _, e := range s.evs {
		if e.ID > id {
			out = append(out, e)
		}
	}
	return out, s.next
}

func (s *Store) ClearEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evs = nil
	_ = os.Remove(path.Join(s.dir, "events.json"))
}

func (s *Store) addEventsLocked(evs []Event) {
	for i := range evs {
		s.next++
		evs[i].ID = s.next
		s.seen[seenKey(evs[i])] = true
	}
	s.evs = append(s.evs, evs...)
	if len(s.evs) > maxEvents {
		s.evs = s.evs[len(s.evs)-maxEvents:]
	}
	b, _ := json.Marshal(s.evs)
	_ = os.WriteFile(path.Join(s.dir, "events.json"), b, 0o600)
}

func (s *Store) baselinePath(alias string) string {
	return path.Join(s.dir, "baseline", alias+".json")
}
func (s *Store) backupDir(alias string) string { return path.Join(s.dir, "backup", alias) }

func (s *Store) loadBaselineLocked(alias string) map[string]entry {
	if m, ok := s.base[alias]; ok {
		return m
	}
	m := map[string]entry{}
	if b, err := os.ReadFile(s.baselinePath(alias)); err == nil {
		_ = json.Unmarshal(b, &m)
	} else {
		return nil
	}
	s.base[alias] = m
	return m
}

func (s *Store) saveBaselineLocked(alias string, m map[string]entry) {
	s.base[alias] = m
	_ = os.MkdirAll(path.Dir(s.baselinePath(alias)), 0o700)
	b, _ := json.Marshal(m)
	_ = os.WriteFile(s.baselinePath(alias), b, 0o600)
}

func excluded(rel string, patterns []string) bool {
	base := path.Base(rel)
	for _, p := range patterns {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
		if ok, _ := path.Match(p, base); ok {
			return true
		}
	}
	return false
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(path.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tamper-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// walk lists regular files under root as site-relative paths.
func walk(root string, excludes []string, fn func(rel string, info fs.FileInfo, full string)) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if excluded(rel, excludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			fn(rel, info, p)
		}
		return nil
	})
}

// Rebuild takes a fresh baseline (and backup copies) of the site root.
func (s *Store) Rebuild(alias, root string) (int, error) {
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return 0, fmt.Errorf("site directory not found: %s", root)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Sites[alias]
	m := map[string]entry{}
	bdir := s.backupDir(alias)
	_ = os.RemoveAll(bdir)
	walk(root, cfg.Excludes, func(rel string, info fs.FileInfo, full string) {
		h, err := hashFile(full)
		if err != nil {
			return
		}
		u, g := owner(info)
		m[rel] = entry{Hash: h, Size: info.Size(), MT: info.ModTime().UnixNano(), Mode: info.Mode().Perm(), UID: u, GID: g}
		if info.Size() <= maxBackupSize {
			_ = copyFile(full, path.Join(bdir, rel), 0o600)
		}
	})
	s.saveBaselineLocked(alias, m)
	cfg.BaselineAt, cfg.Files = time.Now(), len(m)
	s.cfg.Sites[alias] = cfg
	return len(m), s.saveCfgLocked()
}

func (s *Store) HasBaseline(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadBaselineLocked(alias) != nil
}

// Scan compares one site against its baseline; with restore on it also repairs the differences.
func (s *Store) Scan(alias, root string) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Sites[alias]
	base := s.loadBaselineLocked(alias)
	if base == nil {
		return nil
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil
	}
	now := time.Now()
	var found []Event
	current := map[string]bool{}
	walk(root, cfg.Excludes, func(rel string, info fs.FileInfo, full string) {
		current[rel] = true
		old, ok := base[rel]
		if ok && old.Size == info.Size() && old.MT == info.ModTime().UnixNano() {
			return
		}
		h, err := hashFile(full)
		if err != nil {
			return
		}
		switch {
		case !ok:
			found = append(found, Event{Time: now, Site: alias, Kind: "added", Path: rel, Hash: h})
		case h != old.Hash:
			found = append(found, Event{Time: now, Site: alias, Kind: "modified", Path: rel, Hash: h})
		default: // touched but identical
			old.MT = info.ModTime().UnixNano()
			base[rel] = old
		}
	})
	for rel, old := range base {
		if !current[rel] {
			found = append(found, Event{Time: now, Site: alias, Kind: "deleted", Path: rel, Hash: old.Hash})
		}
	}
	var out []Event
	for i := range found {
		e := found[i]
		e.Action = "none"
		if cfg.Restore {
			e.Action = s.repairLocked(alias, root, base, e)
		}
		// repaired changes are always worth logging; unrepaired ones only once
		if e.Action == "none" || e.Action == "restore_failed" {
			if s.seen[seenKey(e)] {
				continue
			}
		}
		out = append(out, e)
	}
	if len(out) > 0 {
		s.addEventsLocked(out)
		// out is a copy of the slice header only; IDs were assigned in place
		for i := range out {
			out[i] = s.evs[len(s.evs)-len(out)+i]
		}
	}
	return out
}

func (s *Store) repairLocked(alias, root string, base map[string]entry, e Event) string {
	full := path.Join(root, e.Path)
	switch e.Kind {
	case "added":
		q := path.Join(s.dir, "quarantine", alias, now14(), e.Path)
		if err := copyFile(full, q, 0o600); err != nil {
			return "restore_failed"
		}
		if err := os.Remove(full); err != nil {
			return "restore_failed"
		}
		return "quarantined"
	default:
		old := base[e.Path]
		bak := path.Join(s.backupDir(alias), e.Path)
		if err := copyFile(bak, full, old.Mode); err != nil {
			return "restore_failed"
		}
		_ = os.Chown(full, old.UID, old.GID)
		_ = os.Chmod(full, old.Mode)
		if info, err := os.Stat(full); err == nil {
			old.MT = info.ModTime().UnixNano()
			base[e.Path] = old
		}
		return "restored"
	}
}

func now14() string { return time.Now().Format("20060102-150405") }

// Run scans every enabled site periodically. sites maps alias to its protected directory.
func (s *Store) Run(sites func() map[string]string) {
	last := time.Time{}
	for {
		time.Sleep(5 * time.Second)
		func() {
			defer func() { _ = recover() }()
			cfg := s.Config()
			if !cfg.Enabled || time.Since(last) < time.Duration(cfg.Interval)*time.Second {
				return
			}
			last = time.Now()
			roots := sites()
			for alias, sc := range cfg.Sites {
				if root, ok := roots[alias]; ok && sc.Enabled {
					s.Scan(alias, root)
				}
			}
		}()
	}
}

func (s *Store) EventCount(site string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.evs {
		if e.Site == site && e.Action != "restored" && e.Action != "quarantined" {
			n++
		}
	}
	return n
}
