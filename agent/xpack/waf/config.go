package waf

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
)

var mu sync.Mutex

var (
	keyPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	cdnHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func validSiteKey(key string) bool {
	if key == "" || key == "." || key == ".." || strings.Contains(key, "..") {
		return false
	}
	return keyPattern.MatchString(key)
}

// allowedRules lists rule files that may be managed through the API.
var allowedRules = map[string]bool{
	"ipBlack": true, "ipWhite": true, "urlBlack": true, "urlWhite": true, "uaBlack": true, "uaWhite": true,
	"args": true, "cookie": true, "header": true, "defaultUaBlack": true, "defaultUrlBlack": true,
	"fileExt": true, "methodWhite": true, "acl": true, "cdn": true, "urlcc": true,
}

var regexRuleFiles = map[string]bool{
	"args": true, "urlBlack": true, "urlWhite": true, "uaBlack": true, "uaWhite": true,
	"cookie": true, "header": true, "defaultUaBlack": true, "defaultUrlBlack": true,
}

var propagatedGlobalRules = map[string]bool{
	"args": true, "cookie": true, "header": true, "acl": true, "urlcc": true, "fileExt": true,
	"cdn": true, "methodWhite": true, "defaultUaBlack": true, "defaultUrlBlack": true,
}

var siteTemplateKeys = []string{
	"args", "cookie", "header", "xss", "sql", "cc", "fileExt",
	"geoRestrict", "cdn", "strict", "defaultUaBlack", "defaultUrlBlack", "methodWhite",
}

type Store struct {
	OpenrestyPath string
	Container     string
}

func (s *Store) dir() string { return dataDir(s.OpenrestyPath) }

type fileSnapshot struct {
	path    string
	existed bool
	data    []byte
	perm    os.FileMode
}

func snapshotFile(p string) fileSnapshot {
	st, err := os.Stat(p)
	if err != nil {
		return fileSnapshot{path: p, existed: false}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return fileSnapshot{path: p, existed: false}
	}
	return fileSnapshot{path: p, existed: true, data: b, perm: st.Mode().Perm()}
}

func restoreSnapshots(snaps []fileSnapshot) {
	seen := map[string]bool{}
	for i := len(snaps) - 1; i >= 0; i-- {
		sn := snaps[i]
		if seen[sn.path] {
			continue
		}
		seen[sn.path] = true
		if !sn.existed {
			_ = os.Remove(sn.path)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(sn.path), 0o755)
		perm := sn.perm
		if perm == 0 {
			perm = 0o644
		}
		tmp := sn.path + ".tmp"
		if os.WriteFile(tmp, sn.data, perm) == nil {
			_ = os.Rename(tmp, sn.path)
		} else {
			_ = os.WriteFile(sn.path, sn.data, perm)
		}
	}
}

func writeJSON(file string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func readJSON(file string, v interface{}) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil
	}
	return json.Unmarshal(content, v)
}

// httpPort discovers the OpenResty HTTP port from <OpenRestyInstallDir>/.env, defaulting to 80.
func (s *Store) httpPort() int {
	if s.OpenrestyPath != "" {
		if data, err := os.ReadFile(path.Join(s.OpenrestyPath, ".env")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "PANEL_APP_PORT_HTTP" {
					v = strings.Trim(strings.TrimSpace(v), `"'`)
					if p, err := strconv.Atoi(v); err == nil && p > 0 && p <= 65535 {
						return p
					}
				}
			}
		}
	}
	return 80
}

func (s *Store) isGlobalWafOn() bool {
	var cfg map[string]interface{}
	if err := readJSON(s.globalFile(), &cfg); err != nil {
		return false
	}
	wafSec, ok := cfg["waf"].(map[string]interface{})
	if !ok {
		return false
	}
	state, _ := wafSec["state"].(string)
	return state == "on"
}

// Reload makes the engine pick up configuration changes.
func (s *Store) Reload() error {
	if s.Container == "" {
		return errors.New("openresty container not found")
	}
	mgr := cmd.NewCommandMgr(cmd.WithTimeout(20 * time.Second))
	if err := mgr.Run("docker", "exec", "-i", s.Container, "nginx", "-t"); err != nil {
		return err
	}
	return mgr.Run("docker", "exec", "-i", s.Container, "nginx", "-s", "reload")
}

// engineToken reads the control token the 1pwaf engine expects on its internal API.
func (s *Store) engineToken() (string, error) {
	content, err := os.ReadFile(path.Join(s.dir(), "conf", "token"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// enginePost calls one of the engine control endpoints from inside the container.
// The engine only accepts these calls from 127.0.0.1, so the request is made with
// curl running inside the OpenResty container itself.
func (s *Store) enginePost(api string, payload map[string]interface{}) (string, error) {
	if s.Container == "" {
		return "", errors.New("openresty container not found")
	}
	token, err := s.engineToken()
	if err != nil {
		return "", err
	}
	body := map[string]interface{}{"token": token}
	for k, v := range payload {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/%s", s.httpPort(), strings.TrimPrefix(api, "/"))
	mgr := cmd.NewCommandMgr(cmd.WithTimeout(20 * time.Second))
	return mgr.RunWithStdout("docker", "exec", s.Container, "curl", "-fsS", "-m", "10",
		"-X", "POST", url, "-H", "Content-Type: application/json", "-d", string(raw))
}

// EngineBlockedIPs lists the IPs the engine currently blocks.
func (s *Store) EngineBlockedIPs() ([]string, error) {
	if !s.isGlobalWafOn() {
		return []string{}, nil
	}
	out, err := s.enginePost("get_block_ip", nil)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "{}" || out == "null" || out == "[]" {
		return []string{}, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(out), &list); err == nil {
		return list, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nil, fmt.Errorf("unexpected engine response: %s", out)
	}
	list = make([]string, 0, len(obj))
	for ip := range obj {
		list = append(list, ip)
	}
	return list, nil
}

// EngineUnblock removes one IP from the engine block list.
func (s *Store) EngineUnblock(ip string) error {
	if strings.TrimSpace(ip) == "" || !s.isGlobalWafOn() {
		return nil
	}
	_, err := s.enginePost("remove_block_ip", map[string]interface{}{"ip": ip})
	return err
}

// EngineUnblockBatch unblocks multiple IPs in a single docker exec invocation, passing JSON payloads via stdin.
func (s *Store) EngineUnblockBatch(ips []string) error {
	if len(ips) == 0 || !s.isGlobalWafOn() {
		return nil
	}
	if s.Container == "" {
		return errors.New("openresty container not found")
	}
	token, err := s.engineToken()
	if err != nil {
		return err
	}
	var stdinBuf bytes.Buffer
	seen := make(map[string]bool, len(ips))
	count := 0
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		raw, err := json.Marshal(map[string]string{"token": token, "ip": ip})
		if err != nil {
			continue
		}
		stdinBuf.Write(raw)
		stdinBuf.WriteByte('\n')
		count++
	}
	if count == 0 {
		return nil
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/remove_block_ip", s.httpPort())
	script := `while IFS= read -r line; do [ -n "$line" ] || continue; curl -fsS -m 10 -X POST "$1" -H "Content-Type: application/json" -d "$line" >/dev/null || exit 1; done`
	timeout := 20 * time.Second
	if extra := time.Duration(count) * 500 * time.Millisecond; extra > timeout {
		if extra > 2*time.Minute {
			extra = 2 * time.Minute
		}
		timeout = extra
	}
	mgr := cmd.NewCommandMgr(cmd.WithTimeout(timeout), cmd.WithStdin(&stdinBuf))
	return mgr.Run("docker", "exec", "-i", s.Container, "sh", "-c", script, "sh", url)
}

func (s *Store) globalFile() string     { return path.Join(s.dir(), "conf", "global.json") }
func (s *Store) siteConfigFile() string { return path.Join(s.dir(), "conf", "siteConfig.json") }
func (s *Store) secretFile() string     { return path.Join(s.dir(), "conf", ".secret") }

func (s *Store) siteFile(key string) (string, error) {
	if !validSiteKey(key) {
		return "", fmt.Errorf("invalid website key: %s", key)
	}
	return path.Join(s.dir(), "sites", key, "config.json"), nil
}

func randomAlnum(n int) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = alphabet[int(time.Now().UnixNano()+int64(i))%len(alphabet)]
		}
		return string(buf)
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

func (s *Store) resolveOrInitSecret() string {
	sf := s.secretFile()
	if b, err := os.ReadFile(sf); err == nil {
		if sec := strings.TrimSpace(string(b)); sec != "" {
			return sec
		}
	}
	sec := randomAlnum(32)
	if st, err := os.Stat(path.Dir(sf)); err == nil && st.IsDir() {
		_ = os.WriteFile(sf, []byte(sec), 0o600)
	}
	return sec
}

func ensureGlobalDefaults(cfg map[string]interface{}, fallbackSecret string) bool {
	changed := false
	wafMap, ok := cfg["waf"].(map[string]interface{})
	if !ok || wafMap == nil {
		wafMap = map[string]interface{}{"state": "on", "mode": "protection"}
		cfg["waf"] = wafMap
		changed = true
	}
	if sec, _ := wafMap["secret"].(string); strings.TrimSpace(sec) == "" && fallbackSecret != "" {
		wafMap["secret"] = fallbackSecret
		changed = true
	}
	if _, ok := cfg["log"].(map[string]interface{}); !ok {
		cfg["log"] = map[string]interface{}{"state": "on", "maxDay": 180, "maxSize": 1}
		changed = true
	}
	if _, ok := cfg["unknownWebsite"].(map[string]interface{}); !ok {
		cfg["unknownWebsite"] = map[string]interface{}{"state": "off", "action": "deny"}
		changed = true
	}
	if _, ok := cfg["notFoundCount"].(map[string]interface{}); !ok {
		cfg["notFoundCount"] = map[string]interface{}{
			"state": "off", "duration": 60, "threshold": 30, "ipBlockTime": 600, "action": "deny",
		}
		changed = true
	}
	if _, ok := cfg["attackCount"].(map[string]interface{}); !ok {
		cfg["attackCount"] = map[string]interface{}{
			"state": "off", "duration": 60, "threshold": 10, "ipBlockTime": 600, "action": "deny",
		}
		changed = true
	}
	if geo, ok := cfg["geoRestrict"].(map[string]interface{}); !ok || geo == nil {
		cfg["geoRestrict"] = map[string]interface{}{
			"state": "off", "action": "deny", "rules": []interface{}{}, "schemaVersion": 2,
		}
		changed = true
	} else if geo["schemaVersion"] == nil {
		geo["schemaVersion"] = 2
		changed = true
	}
	if _, ok := cfg["cdn"].(map[string]interface{}); !ok {
		cfg["cdn"] = map[string]interface{}{"state": "off"}
		changed = true
	}
	return changed
}

func (s *Store) GetGlobal() (map[string]interface{}, error) {
	mu.Lock()
	defer mu.Unlock()
	res := map[string]interface{}{}
	if err := readJSON(s.globalFile(), &res); err != nil {
		return res, err
	}
	sec := s.resolveOrInitSecret()
	if wafMap, ok := res["waf"].(map[string]interface{}); ok {
		if existingSec, _ := wafMap["secret"].(string); strings.TrimSpace(existingSec) == "" {
			wafMap["secret"] = sec
			_ = writeJSON(s.globalFile(), res)
		}
	}
	ensureGlobalDefaults(res, sec)
	return res, nil
}

// UpdateGlobal replaces the given top-level sections of global.json and syncs template keys to siteConfig.json.
func (s *Store) UpdateGlobal(patch map[string]interface{}) error {
	mu.Lock()
	defer mu.Unlock()
	cur := map[string]interface{}{}
	if err := readJSON(s.globalFile(), &cur); err != nil {
		return err
	}
	for k, v := range patch {
		cur[k] = v
	}
	sec := s.resolveOrInitSecret()
	ensureGlobalDefaults(cur, sec)
	if wafMap, ok := cur["waf"].(map[string]interface{}); ok {
		if sVal, _ := wafMap["secret"].(string); strings.TrimSpace(sVal) != "" {
			if st, err := os.Stat(path.Dir(s.secretFile())); err == nil && st.IsDir() {
				_ = os.WriteFile(s.secretFile(), []byte(strings.TrimSpace(sVal)), 0o600)
			}
		}
	}

	snaps := []fileSnapshot{snapshotFile(s.globalFile())}
	if err := writeJSON(s.globalFile(), cur); err != nil {
		return err
	}

	siteCfgPath := s.siteConfigFile()
	siteCfg := map[string]interface{}{}
	if err := readJSON(siteCfgPath, &siteCfg); err == nil || os.IsNotExist(err) {
		if st, statErr := os.Stat(path.Dir(siteCfgPath)); statErr == nil && st.IsDir() {
			snaps = append(snaps, snapshotFile(siteCfgPath))
			if gWaf, ok := cur["waf"].(map[string]interface{}); ok {
				if mode, ok := gWaf["mode"].(string); ok && mode != "" {
					sWaf, ok := siteCfg["waf"].(map[string]interface{})
					if !ok || sWaf == nil {
						sWaf = map[string]interface{}{"state": "on"}
					}
					sWaf["mode"] = mode
					siteCfg["waf"] = sWaf
				}
			}
			for _, k := range siteTemplateKeys {
				if v, ok := cur[k]; ok {
					siteCfg[k] = v
				}
			}
			if err := writeJSON(siteCfgPath, siteCfg); err != nil {
				restoreSnapshots(snaps)
				return err
			}
		}
	}

	if err := s.Reload(); err != nil {
		restoreSnapshots(snaps)
		return err
	}
	return nil
}

type SiteItem struct {
	Key     string                 `json:"key"`
	Domains []string               `json:"domains"`
	Remark  string                 `json:"remark"`
	Config  map[string]interface{} `json:"config"`
}

func (s *Store) existingSiteKeys() []string {
	seen := map[string]bool{}
	var out []string
	var sites []struct {
		Key string `json:"key"`
	}
	if err := readJSON(path.Join(s.dir(), "conf", "sites.json"), &sites); err == nil {
		for _, site := range sites {
			if validSiteKey(site.Key) && !seen[site.Key] {
				if st, err := os.Stat(path.Join(s.dir(), "sites", site.Key)); err == nil && st.IsDir() {
					seen[site.Key] = true
					out = append(out, site.Key)
				}
			}
		}
	}
	if entries, err := os.ReadDir(path.Join(s.dir(), "sites")); err == nil {
		for _, ent := range entries {
			if ent.IsDir() && validSiteKey(ent.Name()) && !seen[ent.Name()] {
				seen[ent.Name()] = true
				out = append(out, ent.Name())
			}
		}
	}
	return out
}

// ApplyToSites copies the given sections of global.json into every site config.
func (s *Store) ApplyToSites(keys []string) error {
	mu.Lock()
	defer mu.Unlock()
	global := map[string]interface{}{}
	if err := readJSON(s.globalFile(), &global); err != nil {
		return err
	}
	var sites []struct {
		Key string `json:"key"`
	}
	if err := readJSON(path.Join(s.dir(), "conf", "sites.json"), &sites); err != nil {
		return err
	}
	var snaps []fileSnapshot
	for _, site := range sites {
		file, err := s.siteFile(site.Key)
		if err != nil {
			continue
		}
		snaps = append(snaps, snapshotFile(file))
		cfg := map[string]interface{}{}
		_ = readJSON(file, &cfg)
		for _, k := range keys {
			if v, ok := global[k]; ok {
				cfg[k] = v
			}
		}
		if err := writeJSON(file, cfg); err != nil {
			restoreSnapshots(snaps)
			return err
		}
	}
	if err := s.Reload(); err != nil {
		restoreSnapshots(snaps)
		return err
	}
	return nil
}

func (s *Store) ListSites() ([]SiteItem, error) {
	var sites []struct {
		Key     string   `json:"key"`
		Domains []string `json:"domains"`
	}
	if err := readJSON(path.Join(s.dir(), "conf", "sites.json"), &sites); err != nil {
		return nil, err
	}
	res := make([]SiteItem, 0, len(sites))
	for _, site := range sites {
		item := SiteItem{Key: site.Key, Domains: site.Domains, Config: map[string]interface{}{}}
		if file, err := s.siteFile(site.Key); err == nil {
			_ = readJSON(file, &item.Config)
		}
		res = append(res, item)
	}
	return res, nil
}

func (s *Store) GetSite(key string) (map[string]interface{}, error) {
	file, err := s.siteFile(key)
	if err != nil {
		return nil, err
	}
	res := map[string]interface{}{}
	return res, readJSON(file, &res)
}

func (s *Store) UpdateSite(key string, patch map[string]interface{}) error {
	file, err := s.siteFile(key)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	cur := map[string]interface{}{}
	if err := readJSON(file, &cur); err != nil {
		return err
	}
	for k, v := range patch {
		cur[k] = v
	}
	snaps := []fileSnapshot{snapshotFile(file)}
	if err := writeJSON(file, cur); err != nil {
		return err
	}
	if err := s.Reload(); err != nil {
		restoreSnapshots(snaps)
		return err
	}
	return nil
}

type Rule map[string]interface{}

type ruleDoc struct {
	Rules []Rule `json:"rules"`
}

func (s *Store) ruleFile(name, site string) (string, error) {
	if !allowedRules[name] {
		return "", fmt.Errorf("unknown rule file: %s", name)
	}
	if site == "" {
		return path.Join(s.dir(), "rules", name+".json"), nil
	}
	if !validSiteKey(site) {
		return "", fmt.Errorf("invalid website key: %s", site)
	}
	return path.Join(s.dir(), "sites", site, "rules", name+".json"), nil
}

func (s *Store) loadRules(name, site string) (string, *ruleDoc, error) {
	file, err := s.ruleFile(name, site)
	if err != nil {
		return "", nil, err
	}
	doc := &ruleDoc{}
	if err := readJSON(file, doc); err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	if doc.Rules == nil {
		doc.Rules = []Rule{}
	}
	return file, doc, nil
}

type RuleListResult struct {
	Total int    `json:"total"`
	Items []Rule `json:"items"`
}

func newRuleID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("r-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func ruleID(r Rule) string {
	if v, ok := r["id"].(string); ok && v != "" {
		return v
	}
	if v, ok := r["rule"].(string); ok && v != "" {
		return v
	}
	v, _ := r["name"].(string)
	return v
}

func cloneRule(r Rule) Rule {
	out := make(Rule, len(r)+1)
	for k, v := range r {
		out[k] = v
	}
	return out
}

func (s *Store) ListRules(name, site, search string, page, size int) (*RuleListResult, error) {
	_, doc, err := s.loadRules(name, site)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	items := make([]Rule, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		if needle == "" || strings.Contains(strings.ToLower(fmt.Sprint(r["rule"], " ", r["name"])), needle) {
			cp := cloneRule(r)
			if id, _ := cp["id"].(string); id == "" {
				cp["id"] = ruleID(cp)
			}
			items = append(items, cp)
		}
	}
	total := len(items)
	if size > 0 {
		start := (page - 1) * size
		if start < 0 {
			start = 0
		}
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}
		items = items[start:end]
	}
	return &RuleListResult{Total: total, Items: items}, nil
}

// normalizeIPRule converts a user supplied IP, CIDR or "start-end" range into the
// entry layout the 1pwaf engine matches against (ipv4 / ipv6 / ipArr).
func normalizeIPRule(rule Rule) error {
	text, _ := rule["rule"].(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("ip is empty")
	}
	for _, k := range []string{"ipv4", "ipv6", "ipStart", "ipEnd", "ipGroup"} {
		delete(rule, k)
	}
	rule["rule"] = text
	if v, _ := rule["name"].(string); v == "" {
		rule["name"] = text
	}
	if rule["state"] == nil {
		rule["state"] = "on"
	}
	switch {
	case strings.Contains(text, "-"):
		parts := strings.SplitN(text, "-", 2)
		start, err1 := netip.ParseAddr(strings.TrimSpace(parts[0]))
		end, err2 := netip.ParseAddr(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || start.Is4() != end.Is4() || end.Less(start) {
			return fmt.Errorf("invalid ip range: %s", text)
		}
		rule["type"], rule["ipStart"], rule["ipEnd"] = "ipArr", start.String(), end.String()
	case strings.Contains(text, "/"):
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return fmt.Errorf("invalid cidr: %s", text)
		}
		prefix = prefix.Masked()
		if prefix.Addr().Is4() {
			first := prefix.Addr()
			last := first.As4()
			for bit := prefix.Bits(); bit < 32; bit++ {
				last[bit/8] |= 1 << (7 - bit%8)
			}
			rule["type"], rule["ipStart"], rule["ipEnd"] = "ipArr", first.String(), netip.AddrFrom4(last).String()
		} else {
			rule["rule"] = prefix.String()
			rule["type"], rule["ipv6"] = "ipv6", prefix.String()
		}
	default:
		addr, err := netip.ParseAddr(text)
		if err != nil {
			return fmt.Errorf("invalid ip: %s", text)
		}
		if addr.Is4() {
			rule["type"], rule["ipv4"] = "ipv4", addr.String()
		} else {
			rule["type"], rule["ipv6"] = "ipv6", addr.String()
		}
	}
	return nil
}

func validateAndNormalizeRule(name string, rule Rule) error {
	switch {
	case name == "ipBlack" || name == "ipWhite":
		return normalizeIPRule(rule)
	case name == "cdn":
		hdr, _ := rule["rule"].(string)
		hdr = strings.ToLower(strings.TrimSpace(hdr))
		if hdr == "" || !cdnHeaderPattern.MatchString(hdr) {
			return fmt.Errorf("invalid cdn header name: %s", hdr)
		}
		rule["rule"] = hdr
		return nil
	case regexRuleFiles[name]:
		pattern, _ := rule["rule"].(string)
		if strings.TrimSpace(pattern) == "" {
			return errors.New("rule pattern is empty")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
		if rule["state"] == nil {
			rule["state"] = "on"
		}
		return nil
	default:
		if rule["conditions"] == nil {
			text, _ := rule["rule"].(string)
			text = strings.TrimSpace(text)
			if text == "" && ruleID(rule) == "" {
				return errors.New("rule is empty")
			}
			if text != "" {
				rule["rule"] = text
			}
		}
		if rule["state"] == nil && name != "cdn" {
			rule["state"] = "on"
		}
		return nil
	}
}

func (s *Store) writeRuleDocAndPropagate(name, site, primaryFile string, doc *ruleDoc) error {
	snaps := []fileSnapshot{snapshotFile(primaryFile)}
	if err := writeJSON(primaryFile, doc); err != nil {
		return err
	}
	if site == "" && propagatedGlobalRules[name] {
		for _, siteKey := range s.existingSiteKeys() {
			sf, err := s.ruleFile(name, siteKey)
			if err != nil {
				continue
			}
			snaps = append(snaps, snapshotFile(sf))
			if err := writeJSON(sf, doc); err != nil {
				restoreSnapshots(snaps)
				return err
			}
		}
	}
	if err := s.Reload(); err != nil {
		restoreSnapshots(snaps)
		return err
	}
	return nil
}

// SaveRule adds or updates (matched by id) a rule, then reloads the engine.
func (s *Store) SaveRule(name, site string, rule Rule, oldID string) error {
	if rule == nil {
		return errors.New("rule is empty")
	}
	incoming := cloneRule(rule)
	if err := validateAndNormalizeRule(name, incoming); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	file, doc, err := s.loadRules(name, site)
	if err != nil {
		return err
	}

	matchIdx := -1
	if oldID != "" {
		for i, r := range doc.Rules {
			if ruleID(r) == oldID {
				matchIdx = i
				break
			}
		}
		if matchIdx < 0 {
			for i, r := range doc.Rules {
				if rv, _ := r["rule"].(string); rv != "" && rv == oldID {
					matchIdx = i
					break
				}
			}
		}
	} else if rv, _ := incoming["rule"].(string); rv != "" {
		for i, r := range doc.Rules {
			if curRule, _ := r["rule"].(string); curRule == rv {
				matchIdx = i
				break
			}
		}
	}

	if matchIdx >= 0 {
		existing := doc.Rules[matchIdx]
		merged := cloneRule(existing)
		oldRuleStr, _ := existing["rule"].(string)
		oldRegStr, hasReg := existing["reg"].(string)
		_, incomingHasReg := incoming["reg"]
		for k, v := range incoming {
			merged[k] = v
		}
		if hasReg && !incomingHasReg && oldRegStr == oldRuleStr {
			if newRuleStr, ok := merged["rule"].(string); ok && newRuleStr != "" {
				merged["reg"] = newRuleStr
			}
		}
		if existingID, _ := existing["id"].(string); existingID != "" {
			merged["id"] = existingID
		} else if curID, _ := merged["id"].(string); curID == "" {
			merged["id"] = newRuleID()
		}
		if err := validateAndNormalizeRule(name, merged); err != nil {
			return err
		}
		doc.Rules[matchIdx] = merged
	} else {
		if id, _ := incoming["id"].(string); id == "" {
			incoming["id"] = newRuleID()
		}
		if ruleID(incoming) == "" {
			return errors.New("rule is empty")
		}
		doc.Rules = append(doc.Rules, incoming)
	}

	return s.writeRuleDocAndPropagate(name, site, file, doc)
}

func (s *Store) DeleteRules(name, site string, ids []string) error {
	mu.Lock()
	defer mu.Unlock()
	file, doc, err := s.loadRules(name, site)
	if err != nil {
		return err
	}
	drop := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			drop[id] = true
		}
	}
	kept := make([]Rule, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		id := ruleID(r)
		rawRule, _ := r["rule"].(string)
		_, hasExplicitID := r["id"].(string)
		if drop[id] || (!hasExplicitID && rawRule != "" && drop[rawRule]) {
			continue
		}
		kept = append(kept, r)
	}
	doc.Rules = kept
	return s.writeRuleDocAndPropagate(name, site, file, doc)
}
