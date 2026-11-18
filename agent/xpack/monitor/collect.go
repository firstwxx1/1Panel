package monitor

import (
	"hash/fnv"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/geo"
	"github.com/oschwald/maxminddb-golang"
)

// Site is one website the collector may read an access log from.
type Site struct {
	Alias   string
	LogPath string
}

// Matches the OpenResty "main" / combined log format, including HTTP 400 "-" / arbitrary request
// strings, optional "$http_x_forwarded_for", optional $request_time, and optional trailing fields
// such as $upstream_response_time.
var lineRe = regexp.MustCompile(`^(\S+)\s+\S+\s+(\S+)\s+\[([^\]]+)\]\s+"([^"]*)"\s+(\d{3})\s+(\d+)\s+"([^"]*)"\s+"([^"]*)"(?:\s+"([^"]*)")?(?:\s+([\d.]+|-))?(?:\s+.*)?$`)

const (
	maxReadPerTick = 8 << 20
	maxGeoCache    = 50000
	geoRetryBackoff = 60 * time.Second
)

type entry struct {
	ts       int64
	ip       string
	country  string
	province string
	method   string
	url      string
	status   int
	bytes    int64
	rt       float64
	hasRT    bool
	ua       string
	referer  string
}

// Collector tails the access logs of enabled websites and folds them into monitor.db.
type Collector struct {
	Store    *Store
	Sites    func() []Site
	GeoPaths func() []string

	geoMu    sync.Mutex
	geoDB    *maxminddb.Reader
	geoPath  string
	geoMtime time.Time
	geoTryAt time.Time
	cache    map[string][2]string
}

func fileInode(st os.FileInfo) int64 {
	if st == nil {
		return 0
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return int64(sys.Ino)
	}
	return 0
}

// Run blocks, collecting every 5 seconds and cleaning up once a day.
func (c *Collector) Run() {
	c.geoMu.Lock()
	if c.cache == nil {
		c.cache = map[string][2]string{}
	}
	c.geoMu.Unlock()

	lastClean := time.Time{}
	for {
		time.Sleep(5 * time.Second)
		func() {
			defer func() { _ = recover() }()
			c.tick()
			if time.Since(lastClean) > 24*time.Hour {
				c.Store.Cleanup()
				lastClean = time.Now()
			}
		}()
	}
}

func (c *Collector) candidateGeoPaths() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if global.Dir.DataDir != "" {
		add(path.Join(global.Dir.DataDir, "geo", "GeoIP.mmdb"))
	}
	add("/opt/1panel/geo/GeoIP.mmdb")
	if c.GeoPaths != nil {
		for _, p := range c.GeoPaths() {
			add(p)
		}
	}
	if global.Dir.AppInstallDir != "" {
		if matches, err := filepath.Glob(filepath.Join(global.Dir.AppInstallDir, "openresty", "*", "1pwaf", "data", "geo", "GeoIP.mmdb")); err == nil {
			for _, m := range matches {
				add(m)
			}
		}
	}
	if matches, err := filepath.Glob("/opt/1panel/apps/openresty/*/1pwaf/data/geo/GeoIP.mmdb"); err == nil {
		for _, m := range matches {
			add(m)
		}
	}
	return out
}

func (c *Collector) refreshGeoLocked() {
	if c.cache == nil {
		c.cache = map[string][2]string{}
	}
	var foundPath string
	var foundStat os.FileInfo
	for _, p := range c.candidateGeoPaths() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
			foundPath = p
			foundStat = st
			break
		}
	}
	if foundPath == "" {
		if c.geoDB != nil {
			_ = c.geoDB.Close()
			c.geoDB = nil
			c.geoPath = ""
			c.geoMtime = time.Time{}
		}
		c.geoTryAt = time.Now()
		return
	}
	if c.geoDB != nil && c.geoPath == foundPath && foundStat.ModTime().Equal(c.geoMtime) {
		return
	}
	reader, err := maxminddb.Open(foundPath)
	if err != nil {
		c.geoTryAt = time.Now()
		return
	}
	if c.geoDB != nil {
		_ = c.geoDB.Close()
	}
	c.geoDB = reader
	c.geoPath = foundPath
	c.geoMtime = foundStat.ModTime()
	c.geoTryAt = time.Time{}
	c.cache = map[string][2]string{}
}

func (c *Collector) tick() {
	cfg := c.Store.Config()
	if !cfg.Enabled {
		return
	}
	c.geoMu.Lock()
	c.refreshGeoLocked()
	c.geoMu.Unlock()

	for _, site := range c.Sites() {
		if !cfg.Sites[site.Alias] {
			continue
		}
		c.collect(site, cfg.For(site.Alias))
	}
}

func (c *Collector) collect(site Site, cfg Config) {
	f, err := os.Open(site.LogPath)
	if err != nil {
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return
	}
	curIno := fileInode(st)

	var off, prevIno int64
	row := c.Store.reader().QueryRow(`SELECT off, COALESCE(ino, 0) FROM offsets WHERE site = ?`, site.Alias)
	if err := row.Scan(&off, &prevIno); err != nil {
		// a freshly enabled site only counts what happens from now on
		c.saveOffset(site.Alias, st.Size(), curIno)
		return
	}
	if (prevIno != 0 && curIno != 0 && curIno != prevIno) || st.Size() < off {
		off = 0 // rotated or truncated
	}
	if st.Size() == off {
		if prevIno == 0 && curIno != 0 {
			c.saveOffset(site.Alias, off, curIno)
		}
		return
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return
	}
	buf, err := io.ReadAll(io.LimitReader(f, maxReadPerTick))
	if err != nil || len(buf) == 0 {
		return
	}
	// only consume complete lines
	end := strings.LastIndexByte(string(buf), '\n')
	if end < 0 {
		return
	}
	data := string(buf[:end+1])
	var entries []entry
	for _, line := range strings.Split(data, "\n") {
		if e, ok := c.parse(line, cfg); ok {
			entries = append(entries, e)
		}
	}
	c.flush(site.Alias, entries, cfg, off+int64(end)+1, curIno)
}

func (c *Collector) saveOffset(site string, off, ino int64) {
	_, _ = c.Store.db.Exec(`INSERT INTO offsets(site, off, ino) VALUES(?, ?, ?) ON CONFLICT(site) DO UPDATE SET off = excluded.off, ino = excluded.ino`, site, off, ino)
}

func suffixOf(rawURL string) string {
	p := rawURL
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 && !strings.Contains(p[i:], "/") {
		return strings.ToLower(p[i+1:])
	}
	return ""
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimPrefix(x, "."), v) {
			return true
		}
	}
	return false
}

func ipExcluded(list []string, ip string) bool {
	parsed := net.ParseIP(ip)
	for _, x := range list {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if x == ip {
			return true
		}
		if _, n, err := net.ParseCIDR(x); err == nil && parsed != nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

func uriExcluded(list []string, p string) bool {
	for _, x := range list {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if strings.HasSuffix(x, "*") {
			if strings.HasPrefix(p, strings.TrimSuffix(x, "*")) {
				return true
			}
		} else if p == x {
			return true
		}
	}
	return false
}

func uaExcluded(list []string, ua string) bool {
	l := strings.ToLower(ua)
	for _, x := range list {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" && strings.Contains(l, x) {
			return true
		}
	}
	return false
}

func cleanClientIP(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.Trim(s, "[]")
	if net.ParseIP(s) != nil {
		return s
	}
	return ""
}

func (c *Collector) parse(line string, cfg Config) (entry, bool) {
	m := lineRe.FindStringSubmatch(line)
	if m == nil {
		return entry{}, false
	}
	t, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[3])
	if err != nil {
		return entry{}, false
	}
	ip := m[1]
	if host, _, err := net.SplitHostPort(ip); err == nil && net.ParseIP(host) != nil {
		ip = host
	}
	if xff := strings.TrimSpace(m[9]); cfg.CDN && xff != "" && xff != "-" {
		if first := cleanClientIP(strings.Split(xff, ",")[0]); first != "" {
			ip = first
		}
	}

	method, reqURL := "-", "-"
	if reqFields := strings.Fields(m[4]); len(reqFields) >= 2 {
		method, reqURL = reqFields[0], reqFields[1]
	} else if len(reqFields) == 1 && reqFields[0] != "-" {
		reqURL = reqFields[0]
	}

	e := entry{ts: t.Unix(), ip: ip, method: method, url: reqURL, ua: m[8], referer: m[7]}
	e.status, _ = strconv.Atoi(m[5])
	e.bytes, _ = strconv.ParseInt(m[6], 10, 64)
	if m[10] != "" && m[10] != "-" {
		if rt, err := strconv.ParseFloat(m[10], 64); err == nil {
			e.rt = rt
			e.hasRT = true
		}
	}
	if ipExcluded(cfg.ExcludeIPs, ip) {
		return entry{}, false
	}
	if len(cfg.ExcludeSuffix) > 0 && inList(cfg.ExcludeSuffix, suffixOf(e.url)) {
		return entry{}, false
	}
	if len(cfg.ExcludeStatus) > 0 && inList(cfg.ExcludeStatus, m[5]) {
		return entry{}, false
	}
	if uriExcluded(cfg.ExcludeUri, urlPath(e.url)) || uaExcluded(cfg.ExcludeUA, e.ua) {
		return entry{}, false
	}
	e.country, e.province = c.locate(ip)
	return e, true
}

func (c *Collector) locate(ip string) (string, string) {
	c.geoMu.Lock()
	defer c.geoMu.Unlock()
	if c.cache == nil {
		c.cache = map[string][2]string{}
	}
	if v, ok := c.cache[ip]; ok {
		return v[0], v[1]
	}
	p := net.ParseIP(ip)
	if p == nil {
		return "未知", ""
	}
	if p.IsPrivate() || p.IsLoopback() || p.IsLinkLocalUnicast() {
		if len(c.cache) >= maxGeoCache {
			c.cache = map[string][2]string{}
		}
		res := [2]string{"内网", ""}
		c.cache[ip] = res
		return res[0], res[1]
	}

	if c.geoDB == nil {
		if c.geoTryAt.IsZero() || time.Since(c.geoTryAt) >= geoRetryBackoff {
			c.refreshGeoLocked()
		}
	}
	if c.geoDB == nil {
		// Do NOT cache empty/unknown lookups when GeoIP DB is unavailable so
		// entries resolve cleanly once GeoIP.mmdb appears on disk.
		return "未知", ""
	}

	res := [2]string{"未知", ""}
	var loc geo.LocationRes
	if err := c.geoDB.Lookup(p, &loc); err == nil && loc.Country.Zh != "" {
		res = [2]string{loc.Country.Zh, loc.Province.Zh}
	}
	if len(c.cache) >= maxGeoCache {
		c.cache = map[string][2]string{}
	}
	c.cache[ip] = res
	return res[0], res[1]
}

var spiders = []struct{ key, name string }{
	{"googlebot", "Google"}, {"bingbot", "Bing"}, {"baiduspider", "Baidu"}, {"yandex", "Yandex"},
	{"sogou", "Sogou"}, {"360spider", "360"}, {"bytespider", "Bytedance"}, {"duckduckbot", "DuckDuckGo"},
	{"applebot", "Apple"}, {"slurp", "Yahoo"}, {"semrush", "Semrush"}, {"ahrefs", "Ahrefs"}, {"petalbot", "Petal"},
	{"bot", "Other"}, {"spider", "Other"}, {"crawler", "Other"},
}

func classifyUA(ua string) (browser, osName, device, spider string) {
	l := strings.ToLower(ua)
	for _, s := range spiders {
		if strings.Contains(l, s.key) {
			spider = s.name
			break
		}
	}
	switch {
	case strings.Contains(l, "micromessenger"):
		browser = "WeChat"
	case strings.Contains(l, "edg/") || strings.Contains(l, "edge/"):
		browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		browser = "Opera"
	case strings.Contains(l, "firefox"):
		browser = "Firefox"
	case strings.Contains(l, "chrome") || strings.Contains(l, "crios"):
		browser = "Chrome"
	case strings.Contains(l, "safari"):
		browser = "Safari"
	case strings.Contains(l, "msie") || strings.Contains(l, "trident"):
		browser = "IE"
	case strings.Contains(l, "curl") || strings.Contains(l, "wget") || strings.Contains(l, "python") || strings.Contains(l, "go-http") || strings.Contains(l, "java/"):
		browser = "Script"
	default:
		browser = "Other"
	}
	switch {
	case strings.Contains(l, "android"):
		osName = "Android"
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad") || strings.Contains(l, "ios"):
		osName = "iOS"
	case strings.Contains(l, "windows"):
		osName = "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		osName = "macOS"
	case strings.Contains(l, "linux"):
		osName = "Linux"
	default:
		osName = "Other"
	}
	switch {
	case spider != "":
		device = "Bot"
	case strings.Contains(l, "ipad") || strings.Contains(l, "tablet"):
		device = "Tablet"
	case strings.Contains(l, "mobile") || strings.Contains(l, "android") || strings.Contains(l, "iphone"):
		device = "Mobile"
	default:
		device = "Desktop"
	}
	return
}

func uaHash(ua string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(ua))
	return int64(h.Sum64() >> 1)
}

func refererHost(ref string) string {
	if ref == "" || ref == "-" {
		return "direct"
	}
	if u, err := url.Parse(ref); err == nil && u.Host != "" {
		return u.Host
	}
	return "other"
}

func urlPath(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if len(raw) > 200 {
		raw = raw[:200]
	}
	return raw
}

type aggRow struct {
	pv, req, bytes, e4, e5, rtcnt int64
	rtsum                         float64
}

type dimKey struct {
	hour int64
	dim  string
	k    string
}

type dimVal struct {
	cnt  int64
	last int64
}

func (c *Collector) flush(site string, entries []entry, cfg Config, newOff, curIno int64) {
	if len(entries) == 0 {
		c.saveOffset(site, newOff, curIno)
		return
	}
	aggs := map[int64]*aggRow{}
	dims := map[dimKey]*dimVal{}
	vis := map[[3]interface{}]int64{}
	dayHour := map[int64]int64{}
	localDay := func(ts int64) int64 {
		if v, ok := dayHour[ts/3600]; ok {
			return v
		}
		v := localDayHour(ts)
		dayHour[ts/3600] = v
		return v
	}
	addDim := func(hour int64, dim, k string, ts int64) {
		if k == "" {
			return
		}
		dk := dimKey{hour, dim, k}
		v := dims[dk]
		if v == nil {
			v = &dimVal{}
			dims[dk] = v
		}
		v.cnt++
		if ts > v.last {
			v.last = ts
		}
	}
	for _, e := range entries {
		hour := e.ts / 3600
		a := aggs[hour]
		if a == nil {
			a = &aggRow{}
			aggs[hour] = a
		}
		a.req++
		a.bytes += e.bytes
		if e.status >= 500 {
			a.e5++
		} else if e.status >= 400 {
			a.e4++
		}
		if e.hasRT {
			a.rtsum += e.rt
			a.rtcnt++
		}
		static := e.url == "-" || inList(cfg.StaticSuffix, suffixOf(e.url))
		if !static {
			a.pv++
			vk := [3]interface{}{localDay(e.ts), e.ip, uaHash(e.ua)}
			if h, ok := vis[vk]; !ok || hour < h {
				vis[vk] = hour
			}
			addDim(hour, "url", urlPath(e.url), e.ts)
			addDim(hour, "referer", refererHost(e.referer), e.ts)
		}
		addDim(hour, "status", strconv.Itoa(e.status), e.ts)
		addDim(hour, "country", e.country, e.ts)
		if e.country == "中国" {
			addDim(hour, "province", e.province, e.ts)
		}
		addDim(localDay(e.ts), "ip", e.ip, e.ts)
		b, o, d, sp := classifyUA(e.ua)
		addDim(hour, "browser", b, e.ts)
		addDim(hour, "os", o, e.ts)
		addDim(hour, "device", d, e.ts)
		if sp != "" {
			addDim(hour, "spider", sp, e.ts)
		}
		if e.status >= 400 {
			addDim(hour, "error", strconv.Itoa(e.status)+" "+urlPath(e.url), e.ts)
		}
	}

	tx, err := c.Store.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	aggStmt, err := tx.Prepare(`INSERT INTO agg(site,hour,pv,req,bytes,e4,e5,rtsum,rtcnt) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(site,hour) DO UPDATE SET pv=pv+excluded.pv, req=req+excluded.req, bytes=bytes+excluded.bytes,
		e4=e4+excluded.e4, e5=e5+excluded.e5, rtsum=rtsum+excluded.rtsum, rtcnt=rtcnt+excluded.rtcnt`)
	if err != nil {
		return
	}
	defer aggStmt.Close()
	for hour, a := range aggs {
		if _, err := aggStmt.Exec(site, hour, a.pv, a.req, a.bytes, a.e4, a.e5, a.rtsum, a.rtcnt); err != nil {
			return
		}
	}

	dimStmt, err := tx.Prepare(`INSERT INTO dim(site,hour,dim,k,cnt,last) VALUES(?,?,?,?,?,?)
		ON CONFLICT(site,hour,dim,k) DO UPDATE SET cnt=cnt+excluded.cnt, last=MAX(last, excluded.last)`)
	if err != nil {
		return
	}
	defer dimStmt.Close()
	for k, v := range dims {
		if _, err := dimStmt.Exec(site, k.hour, k.dim, k.k, v.cnt, v.last); err != nil {
			return
		}
	}

	visStmt, err := tx.Prepare(`INSERT INTO vis2(site,hour,ip,ua,h) VALUES(?,?,?,?,?)
		ON CONFLICT(site,hour,ip,ua) DO UPDATE SET h=MIN(h, excluded.h)`)
	if err != nil {
		return
	}
	defer visStmt.Close()
	for k, h := range vis {
		if _, err := visStmt.Exec(site, k[0], k[1], k[2], h); err != nil {
			return
		}
	}

	// the detail table only keeps the newest rows of this batch; older ones would be trimmed anyway
	rows := entries
	if len(rows) > cfg.DetailMax {
		rows = rows[len(rows)-cfg.DetailMax:]
	}
	hitStmt, err := tx.Prepare(`INSERT INTO hits(site,ts,ip,country,province,method,url,status,bytes,rt,ua,referer) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return
	}
	defer hitStmt.Close()
	for _, e := range rows {
		rt := 0.0
		if e.hasRT {
			rt = e.rt
		}
		if _, err := hitStmt.Exec(site, e.ts, e.ip, e.country, e.province, e.method, e.url, e.status, e.bytes, rt, e.ua, e.referer); err != nil {
			return
		}
	}

	// keep the detail table bounded right away instead of waiting for the daily cleanup
	if _, err := tx.Exec(`DELETE FROM hits WHERE id <= (SELECT MAX(id) FROM hits) - ?`, cfg.DetailMax); err != nil {
		return
	}
	if _, err := tx.Exec(`INSERT INTO offsets(site, off, ino) VALUES(?, ?, ?) ON CONFLICT(site) DO UPDATE SET off = excluded.off, ino = excluded.ino`, site, newOff, curIno); err != nil {
		return
	}
	_ = tx.Commit()
}
