package waf

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The 1pwaf engine normalizes every string into small lookup tables, one sqlite file each.
// SQLite caps ATTACH at 10 databases, so lookups are resolved by id in separate queries.

const (
	maxLookupIDs = 10000
	maxInBatch   = 900
	maxSQLVars   = 30000
)

type dbs struct {
	root string
	open map[string]*sql.DB
}

func newDBs(openrestyPath string) *dbs {
	return &dbs{root: openrestyPath, open: map[string]*sql.DB{}}
}

func (d *dbs) get(group, name string) (*sql.DB, error) {
	key := group + "/" + name
	if db, ok := d.open[key]; ok {
		return db, nil
	}
	db, err := openDB(d.root, group, name)
	if err != nil {
		return nil, err
	}
	d.open[key] = db
	return db, nil
}

func (d *dbs) close() {
	for _, db := range d.open {
		db.Close()
	}
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func idChunks(ids map[int]bool) [][]interface{} {
	if len(ids) == 0 {
		return nil
	}
	all := make([]interface{}, 0, len(ids))
	for id := range ids {
		all = append(all, id)
	}
	var out [][]interface{}
	for i := 0; i < len(all); i += maxInBatch {
		end := i + maxInBatch
		if end > len(all) {
			end = len(all)
		}
		out = append(out, all[i:end])
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// values resolves ids of a "value" lookup table.
func (d *dbs) values(group, name, table string, ids map[int]bool) (map[int]string, error) {
	res := map[int]string{}
	if len(ids) == 0 {
		return res, nil
	}
	db, err := d.get(group, name)
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	for _, chunk := range idChunks(ids) {
		rows, err := db.Query(fmt.Sprintf("SELECT id, value FROM %s WHERE id IN (%s)", table, placeholders(len(chunk))), chunk...)
		if err != nil {
			if isMissingDB(err) {
				return res, nil
			}
			return nil, err
		}
		for rows.Next() {
			var id int
			var v string
			if err := rows.Scan(&id, &v); err != nil {
				rows.Close()
				return nil, err
			}
			res[id] = v
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return res, nil
}

// findIDs returns the ids of lookup rows matching value (exact, or LIKE when fuzzy).
func (d *dbs) findIDs(group, name, table, value string, fuzzy bool) ([]interface{}, error) {
	db, err := d.get(group, name)
	if err != nil {
		if isMissingDB(err) {
			return []interface{}{}, nil
		}
		return nil, err
	}
	query := fmt.Sprintf("SELECT id FROM %s WHERE value = ? LIMIT %d", table, maxLookupIDs)
	arg := value
	if fuzzy {
		query = fmt.Sprintf(`SELECT id FROM %s WHERE value LIKE ? ESCAPE '\' LIMIT %d`, table, maxLookupIDs)
		arg = "%" + escapeLike(value) + "%"
	}
	rows, err := db.Query(query, arg)
	if err != nil {
		if isMissingDB(err) {
			return []interface{}{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	ids := []interface{}{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type location struct{ Country, Province string }

func (d *dbs) locations(ids map[int]bool) (map[int]location, error) {
	res := map[int]location{}
	if len(ids) == 0 {
		return res, nil
	}
	db, err := d.get("global", "location")
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	for _, chunk := range idChunks(ids) {
		rows, err := db.Query("SELECT id, COALESCE(country_zh,''), COALESCE(province_zh,'') FROM locations WHERE id IN ("+placeholders(len(chunk))+")", chunk...)
		if err != nil {
			if isMissingDB(err) {
				return res, nil
			}
			return nil, err
		}
		for rows.Next() {
			var id int
			var l location
			if err := rows.Scan(&id, &l.Country, &l.Province); err != nil {
				rows.Close()
				return nil, err
			}
			res[id] = l
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return res, nil
}

// locationIDs matches ip_location rows by country or province.
func (d *dbs) locationIDs(value string) ([]interface{}, error) {
	db, err := d.get("global", "location")
	if err != nil {
		if isMissingDB(err) {
			return []interface{}{}, nil
		}
		return nil, err
	}
	like := "%" + escapeLike(value) + "%"
	rows, err := db.Query(
		fmt.Sprintf(`SELECT id FROM locations WHERE country_zh LIKE ? ESCAPE '\' OR country_en LIKE ? ESCAPE '\' OR province_zh LIKE ? ESCAPE '\' OR province_en LIKE ? ESCAPE '\' OR iso LIKE ? ESCAPE '\' LIMIT %d`, maxLookupIDs),
		like, like, like, like, like)
	if err != nil {
		if isMissingDB(err) {
			return []interface{}{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	ids := []interface{}{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type LogQuery struct {
	Website  string `json:"website"`
	IP       string `json:"ip"`
	Host     string `json:"host"`
	URL      string `json:"url"`
	Location string `json:"location"`
	RuleType string `json:"ruleType"`
	Action   string `json:"action"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Page     int    `json:"page"`
	Size     int    `json:"size"`
	Export   bool   `json:"export"`
}

type AttackLog struct {
	ID        int    `json:"id"`
	Time      string `json:"time"`
	Website   string `json:"website"`
	Host      string `json:"host"`
	IP        string `json:"ip"`
	Country   string `json:"country"`
	Province  string `json:"province"`
	Method    string `json:"method"`
	URI       string `json:"uri"`
	Rule      string `json:"rule"`
	RuleType  string `json:"ruleType"`
	Match     string `json:"match"`
	Action    string `json:"action"`
	IsBlock   bool   `json:"isBlock"`
	Status    int    `json:"status"`
	UserAgent string `json:"userAgent"`
	Request   string `json:"request"`
}

type AttackLogPage struct {
	Total int         `json:"total"`
	Items []AttackLog `json:"items"`
}

// where builds the attack_logs filter. empty is true when a lookup filter matches nothing.
func (q *LogQuery) where(d *dbs) (cond string, args []interface{}, empty bool, err error) {
	conds := []string{"is_attack = 1"}
	appendIn := func(col string, ids []interface{}) {
		if rem := maxSQLVars - len(args); rem > 0 && len(ids) > rem {
			ids = ids[:rem]
		}
		conds = append(conds, col+" IN ("+placeholders(len(ids))+")")
		args = append(args, ids...)
	}
	add := func(col, group, name, table, value string, fuzzy bool) error {
		if value == "" || empty {
			return nil
		}
		ids, err := d.findIDs(group, name, table, value, fuzzy)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			empty = true
			return nil
		}
		appendIn(col, ids)
		return nil
	}
	if err = add("website_id", "waf", "websites", "websites", q.Website, false); err != nil {
		return
	}
	if err = add("ip_id", "waf", "ips", "ips", q.IP, true); err != nil {
		return
	}
	if err = add("host_id", "waf", "hosts", "hosts", q.Host, true); err != nil {
		return
	}
	if err = add("uri_id", "waf", "uris", "uris", q.URL, true); err != nil {
		return
	}
	if err = add("rule_type_id", "waf", "rule_types", "rule_types", q.RuleType, false); err != nil {
		return
	}
	if err = add("action_id", "global", "default", "actions", q.Action, false); err != nil {
		return
	}
	if q.Location != "" && !empty {
		ids, err := d.locationIDs(q.Location)
		if err != nil {
			return "", nil, false, err
		}
		if len(ids) == 0 {
			return "", nil, true, nil
		}
		appendIn("ip_location_id", ids)
	}
	if q.Start != "" {
		conds = append(conds, "localtime >= ?")
		args = append(args, q.Start)
	}
	if q.End != "" {
		conds = append(conds, "localtime <= ?")
		args = append(args, q.End)
	}
	return strings.Join(conds, " AND "), args, empty, nil
}

type logRow struct {
	id                                                    int
	localtime                                             string
	website, host, ip, rule, ruleType, match, uri, action int
	nginxLog, location, isBlock                           int
}

func SearchAttackLogs(openrestyPath string, q LogQuery) (*AttackLogPage, error) {
	d := newDBs(openrestyPath)
	defer d.close()
	if q.Export {
		q.Page = 1
		q.Size = 100000
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 {
		q.Size = 20
	}
	if q.Size > 100000 {
		q.Size = 100000
	}
	res := &AttackLogPage{Items: []AttackLog{}}
	db, err := d.get("waf", "attack_logs")
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	where, args, empty, err := q.where(d)
	if err != nil || empty {
		return res, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM attack_logs WHERE "+where, args...).Scan(&res.Total); err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	if res.Total == 0 {
		return res, nil
	}
	rows, err := db.Query(`SELECT id, COALESCE(localtime,''), website_id, host_id, ip_id, rule_id, rule_type_id, match_value_id,
 uri_id, action_id, nginx_log_id, ip_location_id, is_block FROM attack_logs WHERE `+where+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(args, q.Size, (q.Page-1)*q.Size)...)
	if err != nil {
		return nil, err
	}
	var list []logRow
	for rows.Next() {
		var r logRow
		if err := rows.Scan(&r.id, &r.localtime, &r.website, &r.host, &r.ip, &r.rule, &r.ruleType, &r.match, &r.uri, &r.action,
			&r.nginxLog, &r.location, &r.isBlock); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()

	ids := func(f func(logRow) int) map[int]bool {
		m := map[int]bool{}
		for _, r := range list {
			m[f(r)] = true
		}
		return m
	}
	sites, err := d.values("waf", "websites", "websites", ids(func(r logRow) int { return r.website }))
	if err != nil {
		return nil, err
	}
	hosts, _ := d.values("waf", "hosts", "hosts", ids(func(r logRow) int { return r.host }))
	ips, _ := d.values("waf", "ips", "ips", ids(func(r logRow) int { return r.ip }))
	rules, _ := d.values("waf", "rules", "rules", ids(func(r logRow) int { return r.rule }))
	types, _ := d.values("waf", "rule_types", "rule_types", ids(func(r logRow) int { return r.ruleType }))
	matches, _ := d.values("waf", "match_values", "match_values", ids(func(r logRow) int { return r.match }))
	uris, _ := d.values("waf", "uris", "uris", ids(func(r logRow) int { return r.uri }))
	actions, _ := d.values("global", "default", "actions", ids(func(r logRow) int { return r.action }))
	locs, _ := d.locations(ids(func(r logRow) int { return r.location }))

	// nginx_logs carries method / ua / status / raw request
	type ngx struct {
		method, ua, status int
		raw                string
	}
	ngxs := map[int]ngx{}
	if nl, err := d.get("waf", "nginx_logs"); err == nil {
		for _, chunk := range idChunks(ids(func(r logRow) int { return r.nginxLog })) {
			if nrows, err := nl.Query("SELECT id, method_id, ua_id, status_code, COALESCE(nginx_log,'') FROM nginx_logs WHERE id IN ("+placeholders(len(chunk))+")", chunk...); err == nil {
				for nrows.Next() {
					var id int
					var n ngx
					if nrows.Scan(&id, &n.method, &n.ua, &n.status, &n.raw) == nil {
						ngxs[id] = n
					}
				}
				nrows.Close()
			}
		}
	}
	methodIDs, uaIDs := map[int]bool{}, map[int]bool{}
	for _, n := range ngxs {
		methodIDs[n.method] = true
		uaIDs[n.ua] = true
	}
	methods, _ := d.values("global", "default", "methods", methodIDs)
	uas, _ := d.values("waf", "user_agents", "user_agents", uaIDs)

	for _, r := range list {
		n := ngxs[r.nginxLog]
		l := AttackLog{
			ID: r.id, Time: r.localtime, Website: sites[r.website], Host: hosts[r.host], IP: ips[r.ip],
			Country: locs[r.location].Country, Province: locs[r.location].Province, Method: methods[n.method],
			URI: uris[r.uri], Rule: rules[r.rule], RuleType: types[r.ruleType], Match: matches[r.match],
			Action: actions[r.action], IsBlock: r.isBlock == 1, Status: n.status, UserAgent: uas[n.ua], Request: n.raw,
		}
		res.Items = append(res.Items, l)
	}
	return res, nil
}

// ClearLogs removes every interception and raw request record kept by the engine.
func ClearLogs(openrestyPath string) error {
	for _, target := range []struct{ dbName, table string }{
		{"attack_logs", "attack_logs"},
		{"nginx_logs", "nginx_logs"},
	} {
		db, err := openDBRW(openrestyPath, "waf", target.dbName)
		if err != nil {
			if isMissingDB(err) {
				continue
			}
			return err
		}
		_, execErr := db.Exec("DELETE FROM " + target.table)
		_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		db.Close()
		if execErr != nil && !isMissingDB(execErr) {
			return execErr
		}
	}
	return nil
}

// ExportLogs renders the filtered interception records as CSV.
func ExportLogs(openrestyPath string, q LogQuery) ([]byte, error) {
	q.Export = true
	page, err := SearchAttackLogs(openrestyPath, q)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	_ = w.Write([]string{"time", "ip", "location", "website", "host", "method", "url", "rule_type", "rule", "matched", "action", "status"})
	for _, it := range page.Items {
		_ = w.Write([]string{it.Time, it.IP, strings.TrimSpace(it.Country + " " + it.Province), it.Website, it.Host,
			it.Method, it.URI, it.RuleType, it.Rule, it.Match, it.Action, strconv.Itoa(it.Status)})
	}
	w.Flush()
	return []byte(sb.String()), w.Error()
}

type LogOptions struct {
	Websites  []string `json:"websites"`
	RuleTypes []string `json:"ruleTypes"`
	Actions   []string `json:"actions"`
}

// LoadLogOptions lists the filter values recorded so far.
func LoadLogOptions(openrestyPath string) (*LogOptions, error) {
	d := newDBs(openrestyPath)
	defer d.close()
	res := &LogOptions{Websites: []string{}, RuleTypes: []string{}, Actions: []string{}}
	read := func(group, name, table string) []string {
		db, err := d.get(group, name)
		if err != nil {
			return []string{}
		}
		rows, err := db.Query("SELECT value FROM " + table + " ORDER BY id")
		if err != nil {
			return []string{}
		}
		defer rows.Close()
		out := []string{}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err == nil && v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	res.Websites = read("waf", "websites", "websites")
	res.RuleTypes = read("waf", "rule_types", "rule_types")
	res.Actions = read("global", "default", "actions")
	return res, nil
}

type Bucket struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Report struct {
	Total     int      `json:"total"`
	Types     []Bucket `json:"types"`
	IPs       []Bucket `json:"ips"`
	Websites  []Bucket `json:"websites"`
	URIs      []Bucket `json:"uris"`
	Countries []Bucket `json:"countries"`
	Provinces []Bucket `json:"provinces"`
}

// topBy groups attack_logs by an id column and resolves names with resolve.
func topBy(db *sql.DB, col, where string, args []interface{},
	resolve func(ids map[int]bool) (map[int]string, error)) ([]Bucket, error) {
	rows, err := db.Query("SELECT "+col+", COUNT(*) c FROM attack_logs WHERE "+where+" GROUP BY "+col+" ORDER BY c DESC LIMIT 10", args...)
	if err != nil {
		return nil, err
	}
	type kv struct{ id, count int }
	var list []kv
	ids := map[int]bool{}
	for rows.Next() {
		var x kv
		if err := rows.Scan(&x.id, &x.count); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, x)
		ids[x.id] = true
	}
	rows.Close()
	names, err := resolve(ids)
	if err != nil {
		return nil, err
	}
	res := []Bucket{}
	for _, x := range list {
		res = append(res, Bucket{Name: names[x.id], Count: x.count})
	}
	return res, nil
}

func sortAndCapBuckets(counts map[string]int, limit int) []Bucket {
	out := make([]Bucket, 0, len(counts))
	for name, cnt := range counts {
		if name == "" || cnt <= 0 {
			continue
		}
		out = append(out, Bucket{Name: name, Count: cnt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func LoadReport(openrestyPath string, q LogQuery) (*Report, error) {
	d := newDBs(openrestyPath)
	defer d.close()
	res := &Report{Types: []Bucket{}, IPs: []Bucket{}, Websites: []Bucket{}, URIs: []Bucket{}, Countries: []Bucket{}, Provinces: []Bucket{}}
	db, err := d.get("waf", "attack_logs")
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	where, args, empty, err := q.where(d)
	if err != nil || empty {
		return res, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM attack_logs WHERE "+where, args...).Scan(&res.Total); err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	if res.Total == 0 {
		return res, nil
	}
	val := func(group, name, table string) func(map[int]bool) (map[int]string, error) {
		return func(ids map[int]bool) (map[int]string, error) { return d.values(group, name, table, ids) }
	}
	if res.Types, err = topBy(db, "rule_type_id", where, args, val("waf", "rule_types", "rule_types")); err != nil {
		return nil, err
	}
	if res.IPs, err = topBy(db, "ip_id", where, args, val("waf", "ips", "ips")); err != nil {
		return nil, err
	}
	if res.Websites, err = topBy(db, "website_id", where, args, val("waf", "websites", "websites")); err != nil {
		return nil, err
	}
	if res.URIs, err = topBy(db, "uri_id", where, args, val("waf", "uris", "uris")); err != nil {
		return nil, err
	}
	res.Countries, res.Provinces, err = topLocations(d, db, where, args)
	return res, err
}

// topLocations aggregates all matching ip_location_id counts by country and Chinese province.
func topLocations(d *dbs, db *sql.DB, where string, args []interface{}) ([]Bucket, []Bucket, error) {
	rows, err := db.Query("SELECT ip_location_id, COUNT(*) FROM attack_logs WHERE "+where+" GROUP BY ip_location_id", args...)
	if err != nil {
		return []Bucket{}, []Bucket{}, err
	}
	counts := map[int]int{}
	ids := map[int]bool{}
	for rows.Next() {
		var id, c int
		if err := rows.Scan(&id, &c); err != nil {
			rows.Close()
			return nil, nil, err
		}
		counts[id] += c
		ids[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	locs, err := d.locations(ids)
	if err != nil {
		return nil, nil, err
	}
	countryCounts := map[string]int{}
	provinceCounts := map[string]int{}
	for id, c := range counts {
		l := locs[id]
		if l.Country != "" {
			countryCounts[l.Country] += c
		}
		if l.Province != "" && (l.Country == "中国" || l.Country == "香港" || l.Country == "澳门" || l.Country == "台湾" || l.Country == "") {
			provinceCounts[l.Province] += c
		}
	}
	return sortAndCapBuckets(countryCounts, 10), sortAndCapBuckets(provinceCounts, 34), nil
}
