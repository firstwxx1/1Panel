package waf

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
)

var mu sync.Mutex

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Rule files that may be managed through the API.
var ruleFiles = map[string]bool{
	"ipBlack": true, "ipWhite": true, "urlBlack": true, "urlWhite": true, "uaBlack": true, "uaWhite": true,
	"args": true, "cookie": true, "header": true, "defaultUaBlack": true, "defaultUrlBlack": true,
	"fileExt": true, "methodWhite": true, "acl": true, "cdn": true, "urlcc": true,
}

type Store struct {
	OpenrestyPath string
	Container     string
}

func (s *Store) dir() string { return dataDir(s.OpenrestyPath) }

func writeJSON(file string, v interface{}) error {
	content, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
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
	mgr := cmd.NewCommandMgr(cmd.WithTimeout(20 * time.Second))
	return mgr.RunWithStdout("docker", "exec", s.Container, "curl", "-s", "-m", "10",
		"-X", "POST", "http://127.0.0.1:8080/"+api, "-H", "Content-Type: application/json", "-d", string(raw))
}

// EngineBlockedIPs lists the IPs the engine currently blocks.
func (s *Store) EngineBlockedIPs() ([]string, error) {
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
	_, err := s.enginePost("remove_block_ip", map[string]interface{}{"ip": ip})
	return err
}

func (s *Store) globalFile() string { return path.Join(s.dir(), "conf", "global.json") }

func (s *Store) siteFile(key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid website key: %s", key)
	}
	return path.Join(s.dir(), "sites", key, "config.json"), nil
}

func (s *Store) GetGlobal() (map[string]interface{}, error) {
	res := map[string]interface{}{}
	return res, readJSON(s.globalFile(), &res)
}

// UpdateGlobal replaces the given top-level sections of global.json.
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
	if err := writeJSON(s.globalFile(), cur); err != nil {
		return err
	}
	return s.Reload()
}

type SiteItem struct {
	Key     string                 `json:"key"`
	Domains []string               `json:"domains"`
	Remark  string                 `json:"remark"`
	Config  map[string]interface{} `json:"config"`
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
	for _, site := range sites {
		file, err := s.siteFile(site.Key)
		if err != nil {
			continue
		}
		cfg := map[string]interface{}{}
		_ = readJSON(file, &cfg)
		for _, k := range keys {
			if v, ok := global[k]; ok {
				cfg[k] = v
			}
		}
		if err := writeJSON(file, cfg); err != nil {
			return err
		}
	}
	return s.Reload()
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
	if err := writeJSON(file, cur); err != nil {
		return err
	}
	return s.Reload()
}

type Rule map[string]interface{}

type ruleDoc struct {
	Rules []Rule `json:"rules"`
}

func (s *Store) ruleFile(name, site string) (string, error) {
	if !ruleFiles[name] {
		return "", fmt.Errorf("unknown rule file: %s", name)
	}
	if site == "" {
		return path.Join(s.dir(), "rules", name+".json"), nil
	}
	if !keyPattern.MatchString(site) {
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

func (s *Store) ListRules(name, site, search string, page, size int) (*RuleListResult, error) {
	_, doc, err := s.loadRules(name, site)
	if err != nil {
		return nil, err
	}
	items := make([]Rule, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		if search == "" || strings.Contains(strings.ToLower(fmt.Sprint(r["rule"], r["name"])), strings.ToLower(search)) {
			items = append(items, r)
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

func ruleID(r Rule) string {
	if v, ok := r["name"].(string); ok && v != "" {
		return v
	}
	v, _ := r["rule"].(string)
	return v
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
		if err1 != nil || err2 != nil || !start.Is4() || !end.Is4() || end.Less(start) {
			return fmt.Errorf("invalid ip range: %s", text)
		}
		rule["type"], rule["ipStart"], rule["ipEnd"] = "ipArr", start.String(), end.String()
	case strings.Contains(text, "/"):
		prefix, err := netip.ParsePrefix(text)
		if err != nil || !prefix.Addr().Is4() {
			return fmt.Errorf("invalid cidr: %s", text)
		}
		prefix = prefix.Masked()
		first := prefix.Addr()
		last := first.As4()
		for bit := prefix.Bits(); bit < 32; bit++ {
			last[bit/8] |= 1 << (7 - bit%8)
		}
		rule["type"], rule["ipStart"], rule["ipEnd"] = "ipArr", first.String(), netip.AddrFrom4(last).String()
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

// SaveRule adds or updates (matched by id) a rule, then reloads the engine.
func (s *Store) SaveRule(name, site string, rule Rule, oldID string) error {
	if name == "ipBlack" || name == "ipWhite" {
		if err := normalizeIPRule(rule); err != nil {
			return err
		}
	}
	if ruleID(rule) == "" {
		return errors.New("rule is empty")
	}
	mu.Lock()
	defer mu.Unlock()
	file, doc, err := s.loadRules(name, site)
	if err != nil {
		return err
	}
	if oldID == "" {
		oldID = ruleID(rule)
	}
	found := false
	for i, r := range doc.Rules {
		if ruleID(r) == oldID {
			doc.Rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		doc.Rules = append(doc.Rules, rule)
	}
	if err := writeJSON(file, doc); err != nil {
		return err
	}
	return s.Reload()
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
		drop[id] = true
	}
	kept := make([]Rule, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		if !drop[ruleID(r)] {
			kept = append(kept, r)
		}
	}
	doc.Rules = kept
	if err := writeJSON(file, doc); err != nil {
		return err
	}
	return s.Reload()
}
