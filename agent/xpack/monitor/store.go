package monitor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/global"
	_ "github.com/glebarez/go-sqlite"
)

// Config is the user-facing monitor configuration, kept in a small json file next to monitor.db.
type Config struct {
	Enabled       bool             `json:"enabled"` // global switch; sites only collect while it is on
	Sites         map[string]bool  `json:"sites"`   // website alias -> enabled
	RetentionDays int              `json:"retentionDays"`
	DetailMax     int              `json:"detailMax"`
	ExcludeIPs    []string         `json:"excludeIps"`
	ExcludeSuffix []string         `json:"excludeSuffix"` // matching requests are dropped entirely
	StaticSuffix  []string         `json:"staticSuffix"`  // matching requests count as traffic but not as page views
	ExcludeStatus []string         `json:"excludeStatus"` // status codes dropped entirely
	ExcludeUri    []string         `json:"excludeUri"`    // url paths dropped entirely; a trailing * matches a prefix
	ExcludeUA     []string         `json:"excludeUa"`     // user-agent substrings dropped entirely
	CDN           bool             `json:"cdn"`           // trust the first X-Forwarded-For entry as the client ip
	SiteRules     map[string]Rules `json:"siteRules"`     // per-website additions to the global rules
}

// Rules are the per-website additions of the exclusion rules; they are merged with the global ones.
type Rules struct {
	ExcludeStatus []string `json:"excludeStatus"`
	ExcludeSuffix []string `json:"excludeSuffix"`
	ExcludeUri    []string `json:"excludeUri"`
	ExcludeIPs    []string `json:"excludeIps"`
	ExcludeUA     []string `json:"excludeUa"`
	CDN           bool     `json:"cdn"`
}

func cloneConfig(src Config) Config {
	out := src
	out.Sites = make(map[string]bool, len(src.Sites))
	for k, v := range src.Sites {
		out.Sites[k] = v
	}
	if src.SiteRules != nil {
		out.SiteRules = make(map[string]Rules, len(src.SiteRules))
		for k, v := range src.SiteRules {
			out.SiteRules[k] = v
		}
	}
	return out
}

// For returns the effective configuration of one website: global rules plus its own.
func (c Config) For(alias string) Config {
	r, ok := c.SiteRules[alias]
	if !ok {
		return c
	}
	join := func(a, b []string) []string { return append(append([]string{}, a...), b...) }
	c.ExcludeStatus = join(c.ExcludeStatus, r.ExcludeStatus)
	c.ExcludeSuffix = join(c.ExcludeSuffix, r.ExcludeSuffix)
	c.ExcludeUri = join(c.ExcludeUri, r.ExcludeUri)
	c.ExcludeIPs = join(c.ExcludeIPs, r.ExcludeIPs)
	c.ExcludeUA = join(c.ExcludeUA, r.ExcludeUA)
	c.CDN = c.CDN || r.CDN
	return c
}

func defaultConfig() Config {
	return Config{
		Sites:         map[string]bool{},
		RetentionDays: 30,
		DetailMax:     200000,
		ExcludeIPs:    []string{},
		ExcludeSuffix: []string{},
		StaticSuffix:  []string{"js", "css", "png", "jpg", "jpeg", "gif", "ico", "svg", "webp", "woff", "woff2", "ttf", "eot", "map", "mp4", "mp3"},
	}
}

// Store owns monitor.db. It is created lazily by Open and shared by the collector and the API.
type Store struct {
	Dir string // <data dir>/monitor
	db  *sql.DB
	rdb *sql.DB

	mu  sync.Mutex
	cfg *Config
}

var (
	defaultStore *Store
	storeMu      sync.Mutex
)

// Open returns the process-wide store rooted at dir.
func Open(dir string) (*Store, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if defaultStore != nil {
		return defaultStore, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dbFile := path.Join(dir, "monitor.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbFile))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &Store{Dir: dir, db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	rdb, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=query_only(ON)", dbFile))
	if err == nil {
		rdb.SetMaxOpenConns(4)
		s.rdb = rdb
	}

	defaultStore = s
	return s, nil
}

// DefaultStore returns the active monitor store if already opened or present on disk.
func DefaultStore() (*Store, error) {
	storeMu.Lock()
	if defaultStore != nil {
		s := defaultStore
		storeMu.Unlock()
		return s, nil
	}
	storeMu.Unlock()

	if global.Dir.DataDir == "" {
		return nil, errors.New("data dir not initialized")
	}
	dir := path.Join(global.Dir.DataDir, "monitor")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, nil
	}
	return Open(dir)
}

func (s *Store) reader() *sql.DB {
	if s.rdb != nil {
		return s.rdb
	}
	return s.db
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS hits (
			id INTEGER PRIMARY KEY AUTOINCREMENT, site TEXT, ts INTEGER, ip TEXT, country TEXT, province TEXT,
			method TEXT, url TEXT, status INTEGER, bytes INTEGER, rt REAL, ua TEXT, referer TEXT)`,
		`CREATE INDEX IF NOT EXISTS idx_hits_site ON hits(site, id)`,
		`CREATE INDEX IF NOT EXISTS idx_hits_ts ON hits(ts)`,
		`CREATE TABLE IF NOT EXISTS agg (
			site TEXT, hour INTEGER, pv INTEGER, req INTEGER, bytes INTEGER, e4 INTEGER, e5 INTEGER, rtsum REAL, rtcnt INTEGER,
			PRIMARY KEY (site, hour))`,
		`CREATE TABLE IF NOT EXISTS dim (
			site TEXT, hour INTEGER, dim TEXT, k TEXT, cnt INTEGER, last INTEGER,
			PRIMARY KEY (site, hour, dim, k))`,
		`DROP TABLE IF EXISTS vis`,
		// visitors and ips are deduplicated per local day (hour holds the day's first hour) to keep the db small
		// h is the hour the visitor was first seen that day, so hourly new-visitor counts add up to the day's UV
		`CREATE TABLE IF NOT EXISTS vis2 (site TEXT, hour INTEGER, ip TEXT, ua INTEGER, h INTEGER, PRIMARY KEY (site, hour, ip, ua)) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS offsets (site TEXT PRIMARY KEY, off INTEGER, ino INTEGER DEFAULT 0)`,
	}
	// vis2 without the h column comes from an older build; its numbers cannot be upgraded
	var hasH, exists int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('vis2') WHERE name = 'h'`).Scan(&hasH)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'vis2'`).Scan(&exists)
	if exists > 0 && hasH == 0 {
		_, _ = s.db.Exec(`DROP TABLE vis2`)
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// Migrate existing offsets tables created without the ino column (ignore duplicate-column error).
	_, _ = s.db.Exec(`ALTER TABLE offsets ADD COLUMN ino INTEGER DEFAULT 0`)
	return nil
}

func (s *Store) cfgFile() string { return path.Join(s.Dir, "config.json") }

func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg == nil {
		c := defaultConfig()
		if data, err := os.ReadFile(s.cfgFile()); err == nil {
			_ = json.Unmarshal(data, &c)
		}
		if c.Sites == nil {
			c.Sites = map[string]bool{}
		}
		if c.RetentionDays <= 0 {
			c.RetentionDays = 30
		}
		if c.DetailMax <= 0 {
			c.DetailMax = 200000
		}
		s.cfg = &c
	}
	return cloneConfig(*s.cfg)
}

func (s *Store) SaveConfig(c Config) error {
	if c.RetentionDays <= 0 {
		c.RetentionDays = 30
	}
	if c.DetailMax <= 0 {
		c.DetailMax = 200000
	}
	if c.Sites == nil {
		c.Sites = map[string]bool{}
	}
	cloned := cloneConfig(c)
	data, err := json.MarshalIndent(cloned, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.cfgFile()), 0o755); err != nil {
		return err
	}
	tmp := s.cfgFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.cfgFile()); err != nil {
		return err
	}
	s.cfg = &cloned
	return nil
}

// RemoveSite cleans up config entries and all SQLite tables for a deleted website.
func (s *Store) RemoveSite(alias string) error {
	if alias == "" {
		return nil
	}
	cfg := s.Config()
	cfgChanged := false
	if _, ok := cfg.Sites[alias]; ok {
		delete(cfg.Sites, alias)
		cfgChanged = true
	}
	if cfg.SiteRules != nil {
		if _, ok := cfg.SiteRules[alias]; ok {
			delete(cfg.SiteRules, alias)
			cfgChanged = true
		}
	}
	if cfgChanged {
		_ = s.SaveConfig(cfg)
	}
	for _, t := range []string{"hits", "agg", "dim", "vis2", "offsets"} {
		if _, err := s.db.Exec("DELETE FROM "+t+" WHERE site = ?", alias); err != nil {
			return err
		}
	}
	return nil
}

// Cleanup removes data older than the retention window, trims the detail table, and checkpoints WAL.
func (s *Store) Cleanup() {
	c := s.Config()
	cut := time.Now().AddDate(0, 0, -c.RetentionDays)
	hour := cut.Unix() / 3600
	_, _ = s.db.Exec(`DELETE FROM agg WHERE hour < ?`, hour)
	_, _ = s.db.Exec(`DELETE FROM dim WHERE hour < ?`, hour)
	_, _ = s.db.Exec(`DELETE FROM vis2 WHERE hour < ?`, hour)
	_, _ = s.db.Exec(`DELETE FROM hits WHERE ts < ?`, cut.Unix())
	_, _ = s.db.Exec(`DELETE FROM hits WHERE id <= (SELECT MAX(id) FROM hits) - ?`, c.DetailMax)
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
}

// Clear drops every collected number of a website (or all when site is empty).
func (s *Store) Clear(site string) error {
	for _, t := range []string{"hits", "agg", "dim", "vis2"} {
		q := "DELETE FROM " + t
		var args []interface{}
		if site != "" {
			q += " WHERE site = ?"
			args = append(args, site)
		}
		if _, err := s.db.Exec(q, args...); err != nil {
			return err
		}
	}
	return nil
}

// ForgetOffset makes a site start from the end of its log the next time it is enabled.
func (s *Store) ForgetOffset(site string) {
	_, _ = s.db.Exec(`DELETE FROM offsets WHERE site = ?`, site)
}
