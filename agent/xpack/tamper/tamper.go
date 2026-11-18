package tamper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/agent/global"
)

// Web tamper protection: a baseline (hash of every file) is taken for each protected site and
// scanned against periodically. Changes are logged and, optionally, undone from a private copy.

const (
	maxBackupSize = 10 << 20 // bigger files are hashed but not copied, so they cannot be restored
	maxEvents     = 2000
)

var DefaultExcludes = []string{".git", "node_modules", "runtime", "cache", "*.log"}

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
	dir       string
	mu        sync.Mutex
	siteLocks map[string]*sync.Mutex
	cfg       Config
	evs       []Event
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
	s := &Store{
		dir:       dir,
		siteLocks: map[string]*sync.Mutex{},
		cfg:       Config{Interval: 60, Sites: map[string]SiteCfg{}},
		seen:      map[string]bool{},
		base:      map[string]map[string]entry{},
	}
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
		if e.Action == "none" || e.Action == "restore_failed" {
			s.seen[seenKey(e)] = true
		}
		if e.ID > s.next {
			s.next = e.ID
		}
	}
	stores[dir] = s
	return s, nil
}

// DefaultStore returns the active tamper store if already opened or present on disk.
func DefaultStore() (*Store, error) {
	if global.Dir.DataDir == "" {
		return nil, errors.New("data dir not initialized")
	}
	dir := path.Join(global.Dir.DataDir, "tamper")
	storeMu.Lock()
	if s, ok := stores[dir]; ok {
		storeMu.Unlock()
		return s, nil
	}
	storeMu.Unlock()

	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, nil
	}
	return Open(dir)
}

func (s *Store) lockSite(alias string) func() {
	s.mu.Lock()
	if s.siteLocks == nil {
		s.siteLocks = map[string]*sync.Mutex{}
	}
	m, ok := s.siteLocks[alias]
	if !ok {
		m = &sync.Mutex{}
		s.siteLocks[alias] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func seenKey(e Event) string { return e.Site + "|" + e.Kind + "|" + e.Path + "|" + e.Hash }

func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Sites = map[string]SiteCfg{}
	for k, v := range s.cfg.Sites {
		cp := v
		if v.Excludes != nil {
			cp.Excludes = append([]string(nil), v.Excludes...)
		}
		c.Sites[k] = cp
	}
	return c
}

func (s *Store) saveCfgLocked() error {
	b, _ := json.MarshalIndent(s.cfg, "", "  ")
	tmp := path.Join(s.dir, "config.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path.Join(s.dir, "config.json"))
}

func (s *Store) saveEventsLocked() {
	if len(s.evs) == 0 {
		_ = os.Remove(path.Join(s.dir, "events.json"))
		return
	}
	b, _ := json.Marshal(s.evs)
	tmp := path.Join(s.dir, "events.json.tmp")
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, path.Join(s.dir, "events.json"))
	}
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
	c, exists := s.cfg.Sites[alias]
	c.Enabled, c.Restore = enabled, restore
	var clean []string
	for _, e := range excludes {
		if e = strings.TrimSpace(e); e != "" {
			clean = append(clean, e)
		}
	}
	if !exists && len(clean) == 0 {
		clean = append([]string(nil), DefaultExcludes...)
	}
	c.Excludes = clean
	s.cfg.Sites[alias] = c
	return s.saveCfgLocked()
}

// RemoveSite removes all tamper state, baselines, backups, and quarantined files for a deleted website.
func (s *Store) RemoveSite(alias string) error {
	if alias == "" {
		return nil
	}
	unlock := s.lockSite(alias)
	defer unlock()

	s.mu.Lock()
	delete(s.cfg.Sites, alias)
	delete(s.base, alias)
	prefix := alias + "|"
	for k := range s.seen {
		if strings.HasPrefix(k, prefix) {
			delete(s.seen, k)
		}
	}
	kept := s.evs[:0]
	evChanged := false
	for _, e := range s.evs {
		if e.Site == alias {
			evChanged = true
			continue
		}
		kept = append(kept, e)
	}
	s.evs = kept
	if evChanged {
		s.saveEventsLocked()
	}
	err := s.saveCfgLocked()
	bfile := s.baselinePath(alias)
	bdir := s.backupDir(alias)
	qdir := s.quarantineDir(alias)
	s.mu.Unlock()

	_ = os.Remove(bfile)
	_ = os.RemoveAll(bdir)
	_ = os.RemoveAll(qdir)
	return err
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
	s.evs = nil
	s.seen = map[string]bool{}
	_ = os.Remove(path.Join(s.dir, "events.json"))
	qdir := path.Join(s.dir, "quarantine")
	s.mu.Unlock()
	_ = os.RemoveAll(qdir)
}

func (s *Store) addEventsLocked(evs []Event) {
	for i := range evs {
		s.next++
		evs[i].ID = s.next
		if evs[i].Action == "none" || evs[i].Action == "restore_failed" {
			s.seen[seenKey(evs[i])] = true
		}
	}
	s.evs = append(s.evs, evs...)
	if len(s.evs) > maxEvents {
		s.evs = s.evs[len(s.evs)-maxEvents:]
	}
	s.saveEventsLocked()
}

func (s *Store) baselinePath(alias string) string {
	return path.Join(s.dir, "baseline", alias+".json")
}
func (s *Store) backupDir(alias string) string     { return path.Join(s.dir, "backup", alias) }
func (s *Store) quarantineDir(alias string) string { return path.Join(s.dir, "quarantine", alias) }

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
	tmp := s.baselinePath(alias) + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, s.baselinePath(alias))
	} else {
		_ = os.WriteFile(s.baselinePath(alias), b, 0o600)
	}
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

func hashRootFile(r *os.Root, rel string) (string, error) {
	info, err := r.Lstat(rel)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", rel)
	}
	f, err := r.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", rel)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFromRoot copies a regular file from inside r to a private host destination, bounded by maxBytes.
func copyFromRoot(r *os.Root, rel, dst string, mode os.FileMode, maxBytes int64) error {
	if err := os.MkdirAll(path.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := r.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", rel)
	}
	if maxBytes > 0 && st.Size() > maxBytes {
		return fmt.Errorf("file exceeds max size: %d", st.Size())
	}
	tmp := dst + ".tamper-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	var src io.Reader = in
	if maxBytes > 0 {
		src = io.LimitReader(in, maxBytes+1)
	}
	n, err := io.Copy(out, src)
	if err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if maxBytes > 0 && n > maxBytes {
		out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("file exceeds max size")
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func mkdirAllRoot(r *os.Root, relDir string, perm os.FileMode) error {
	relDir = strings.Trim(path.Clean(relDir), "/")
	if relDir == "" || relDir == "." {
		return nil
	}
	parts := strings.Split(relDir, "/")
	cur := ""
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if cur == "" {
			cur = p
		} else {
			cur = cur + "/" + p
		}
		st, err := r.Lstat(cur)
		if err == nil {
			if !st.IsDir() {
				if err := r.Remove(cur); err != nil {
					return err
				}
				if err := r.Mkdir(cur, perm); err != nil {
					return err
				}
			}
			continue
		}
		if err := r.Mkdir(cur, perm); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// restoreToRoot copies a clean backup file back into website root via r without following symlinks
// or allowing TOCTOU on Chmod / Chown.
func restoreToRoot(r *os.Root, bakPath, rel string, old entry) (os.FileInfo, error) {
	in, err := os.Open(bakPath)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	if err := mkdirAllRoot(r, path.Dir(rel), 0o755); err != nil {
		return nil, err
	}
	if st, err := r.Lstat(rel); err == nil && st.IsDir() {
		if err := r.RemoveAll(rel); err != nil {
			return nil, err
		}
	}

	tmpRel := rel + ".tamper-tmp"
	_ = r.Remove(tmpRel)
	mode := old.Mode
	if mode == 0 {
		mode = 0o644
	}
	out, err := r.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = r.Remove(tmpRel)
		return nil, err
	}
	_ = out.Chmod(mode)
	if lst, err := r.Lstat(tmpRel); err != nil || !lst.Mode().IsRegular() {
		out.Close()
		_ = r.Remove(tmpRel)
		return nil, errors.New("unexpected non-regular target during restore")
	}
	if old.UID >= 0 && old.GID >= 0 {
		_ = out.Chown(old.UID, old.GID)
	}
	if err := out.Close(); err != nil {
		_ = r.Remove(tmpRel)
		return nil, err
	}
	if err := r.Rename(tmpRel, rel); err != nil {
		_ = r.Remove(tmpRel)
		return nil, err
	}
	_ = r.Chmod(rel, mode)
	return r.Lstat(rel)
}

// walk traverses r safely via r.FS() (never escaping root), invoking onFile for regular files
// and onSymlink for unexpected symlinks.
func walk(r *os.Root, excludes []string, onFile func(rel string, info fs.FileInfo), onSymlink func(rel string)) {
	_ = fs.WalkDir(r.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if rel == "." || strings.HasSuffix(rel, ".tamper-tmp") {
			return nil
		}
		if excluded(rel, excludes) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if onSymlink != nil {
				onSymlink(rel)
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if info, err := r.Lstat(rel); err == nil && info.Mode().IsRegular() {
			onFile(rel, info)
		}
		return nil
	})
}

// Rebuild takes a fresh baseline (and backup copies) of the site root without holding s.mu during I/O.
func (s *Store) Rebuild(alias, root string) (int, error) {
	unlock := s.lockSite(alias)
	defer unlock()

	r, err := os.OpenRoot(root)
	if err != nil {
		return 0, fmt.Errorf("site directory not found: %s", root)
	}
	defer r.Close()

	s.mu.Lock()
	cfg, exists := s.cfg.Sites[alias]
	if !exists && len(cfg.Excludes) == 0 {
		cfg.Excludes = append([]string(nil), DefaultExcludes...)
	}
	excludes := append([]string(nil), cfg.Excludes...)
	bdir := s.backupDir(alias)
	s.mu.Unlock()

	stageDir := bdir + ".rebuilding"
	_ = os.RemoveAll(stageDir)
	m := map[string]entry{}
	walk(r, excludes, func(rel string, info fs.FileInfo) {
		h, err := hashRootFile(r, rel)
		if err != nil {
			return
		}
		u, g := owner(info)
		m[rel] = entry{Hash: h, Size: info.Size(), MT: info.ModTime().UnixNano(), Mode: info.Mode().Perm(), UID: u, GID: g}
		if info.Size() <= maxBackupSize {
			_ = copyFromRoot(r, rel, path.Join(stageDir, rel), 0o600, maxBackupSize)
		}
	}, nil)

	_ = os.RemoveAll(bdir)
	if _, err := os.Stat(stageDir); err == nil {
		_ = os.MkdirAll(path.Dir(bdir), 0o700)
		_ = os.Rename(stageDir, bdir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	curCfg, curExists := s.cfg.Sites[alias]
	if !curExists && len(curCfg.Excludes) == 0 {
		curCfg.Excludes = excludes
	}
	curCfg.BaselineAt, curCfg.Files = time.Now(), len(m)
	s.cfg.Sites[alias] = curCfg
	s.saveBaselineLocked(alias, m)

	// Clear seen entries and pending unresolved events for this site on baseline rebuild.
	prefix := alias + "|"
	for k := range s.seen {
		if strings.HasPrefix(k, prefix) {
			delete(s.seen, k)
		}
	}
	kept := s.evs[:0]
	evChanged := false
	for _, e := range s.evs {
		if e.Site == alias && (e.Action == "none" || e.Action == "restore_failed") {
			evChanged = true
			continue
		}
		kept = append(kept, e)
	}
	s.evs = kept
	if evChanged {
		s.saveEventsLocked()
	}

	return len(m), s.saveCfgLocked()
}

func (s *Store) HasBaseline(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadBaselineLocked(alias) != nil
}

// Scan compares one site against its baseline; with restore on it also repairs the differences.
func (s *Store) Scan(alias, root string) []Event {
	unlock := s.lockSite(alias)
	defer unlock()

	s.mu.Lock()
	cfg := s.cfg.Sites[alias]
	if !cfg.Enabled { // a disabled site must never be touched, not even by a manual scan
		s.mu.Unlock()
		return nil
	}
	origBase := s.loadBaselineLocked(alias)
	if origBase == nil {
		s.mu.Unlock()
		return nil
	}
	base := make(map[string]entry, len(origBase))
	for k, v := range origBase {
		base[k] = v
	}
	excludes := append([]string(nil), cfg.Excludes...)
	restoreOn := cfg.Restore
	s.mu.Unlock()

	r, err := os.OpenRoot(root)
	if err != nil {
		return nil
	}
	defer r.Close()

	now := time.Now()
	var found []Event
	current := map[string]bool{}
	walk(r, excludes, func(rel string, info fs.FileInfo) {
		current[rel] = true
		old, ok := base[rel]
		if ok && old.Size == info.Size() && old.MT == info.ModTime().UnixNano() {
			return
		}
		h, err := hashRootFile(r, rel)
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
	}, func(rel string) {
		if _, inBase := base[rel]; inBase {
			current[rel] = true
			found = append(found, Event{Time: now, Site: alias, Kind: "modified", Path: rel, Hash: "symlink"})
		} else {
			found = append(found, Event{Time: now, Site: alias, Kind: "added", Path: rel, Hash: "symlink"})
		}
	})

	for rel, old := range base {
		if !current[rel] {
			found = append(found, Event{Time: now, Site: alias, Kind: "deleted", Path: rel, Hash: old.Hash})
		}
	}

	stillTampered := map[string]bool{}
	for i := range found {
		found[i].Action = "none"
		if restoreOn {
			found[i].Action = s.repairLocked(alias, r, base, found[i])
		}
		if found[i].Action == "none" || found[i].Action == "restore_failed" {
			stillTampered[found[i].Path] = true
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if curCfg, ok := s.cfg.Sites[alias]; !ok || !curCfg.Enabled {
		return nil
	}
	s.base[alias] = base

	// Clear s.seen entries for (alias, path) as soon as a path matches baseline again.
	prefix := alias + "|"
	for k := range s.seen {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		parts := strings.SplitN(k, "|", 4)
		if len(parts) >= 3 && !stillTampered[parts[2]] {
			delete(s.seen, k)
		}
	}

	var out []Event
	for _, e := range found {
		if e.Action == "none" || e.Action == "restore_failed" {
			if s.seen[seenKey(e)] {
				continue
			}
		}
		out = append(out, e)
	}
	if len(out) > 0 {
		s.addEventsLocked(out)
		for i := range out {
			out[i] = s.evs[len(s.evs)-len(out)+i]
		}
	}
	return out
}

func (s *Store) repairLocked(alias string, r *os.Root, base map[string]entry, e Event) string {
	switch e.Kind {
	case "added":
		st, err := r.Lstat(e.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return "quarantined"
			}
			return "restore_failed"
		}
		if st.Mode()&os.ModeSymlink != 0 {
			if err := r.Remove(e.Path); err != nil {
				return "restore_failed"
			}
			return "quarantined"
		}
		if st.Mode().IsRegular() && st.Size() <= maxBackupSize {
			q := path.Join(s.quarantineDir(alias), now14(), e.Path)
			if err := copyFromRoot(r, e.Path, q, 0o600, maxBackupSize); err != nil {
				// If file grew past maxBackupSize while copying, proceed with removal anyway.
				if curSt, stErr := r.Lstat(e.Path); stErr != nil || curSt.Size() <= maxBackupSize {
					return "restore_failed"
				}
			}
		}
		if err := r.Remove(e.Path); err != nil {
			return "restore_failed"
		}
		return "quarantined"
	default:
		old := base[e.Path]
		bak := path.Join(s.backupDir(alias), e.Path)
		info, err := restoreToRoot(r, bak, e.Path, old)
		if err != nil {
			return "restore_failed"
		}
		old.MT = info.ModTime().UnixNano()
		base[e.Path] = old
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
