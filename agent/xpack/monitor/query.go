package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RangeReq struct {
	Site  string `json:"site"` // empty = all websites
	Range string `json:"range"`
	Start int64  `json:"start"` // unix seconds; with End it overrides Range
	End   int64  `json:"end"`
}

// window resolves a named range to [start, end) in unix seconds plus the equally long previous window.
func window(name string, from, to int64) (start, end, pstart int64, byDay bool) {
	if from > 0 && to > from {
		span := to - from
		return from, to, from - span, span > 25*3600
	}
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch name {
	case "yesterday":
		start, end = day.AddDate(0, 0, -1).Unix(), day.Unix()
		pstart = day.AddDate(0, 0, -2).Unix()
	case "7d":
		start, end = day.AddDate(0, 0, -6).Unix(), day.AddDate(0, 0, 1).Unix()
		pstart = day.AddDate(0, 0, -13).Unix()
		byDay = true
	case "30d":
		start, end = day.AddDate(0, 0, -29).Unix(), day.AddDate(0, 0, 1).Unix()
		pstart = day.AddDate(0, 0, -59).Unix()
		byDay = true
	default:
		start, end = day.Unix(), day.AddDate(0, 0, 1).Unix()
		pstart = day.AddDate(0, 0, -1).Unix()
	}
	return
}

func siteClause(site string) (string, []interface{}) {
	if site == "" {
		return "", nil
	}
	return " AND site = ?", []interface{}{site}
}

type Totals struct {
	PV     int64   `json:"pv"`
	UV     int64   `json:"uv"`
	IP     int64   `json:"ip"`
	Req    int64   `json:"req"`
	Bytes  int64   `json:"bytes"`
	Err4   int64   `json:"err4"`
	Err5   int64   `json:"err5"`
	AvgRT  float64 `json:"avgRt"` // seconds, -1 when the log format carries no request time
	Spiders int64  `json:"spiders"`
}

func (s *Store) totals(site string, from, to int64) (Totals, error) {
	sc, sa := siteClause(site)
	h0, h1 := from/3600, (to+3599)/3600
	var t Totals
	var rtsum float64
	var rtcnt int64
	args := append([]interface{}{h0, h1}, sa...)
	row := s.db.QueryRow(`SELECT COALESCE(SUM(pv),0),COALESCE(SUM(req),0),COALESCE(SUM(bytes),0),COALESCE(SUM(e4),0),COALESCE(SUM(e5),0),
		COALESCE(SUM(rtsum),0),COALESCE(SUM(rtcnt),0) FROM agg WHERE hour >= ? AND hour < ?`+sc, args...)
	if err := row.Scan(&t.PV, &t.Req, &t.Bytes, &t.Err4, &t.Err5, &rtsum, &rtcnt); err != nil {
		return t, err
	}
	t.AvgRT = -1
	if rtcnt > 0 {
		t.AvgRT = rtsum / float64(rtcnt)
	}
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT ip), COUNT(*) FROM (SELECT DISTINCT ip, ua FROM vis2 WHERE hour >= ? AND hour < ?`+sc+`)`, args...).Scan(&t.IP, &t.UV); err != nil {
		return t, err
	}
	// ip count should also include static-only visitors; the ip dimension has them
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT k) FROM dim WHERE dim = 'ip' AND hour >= ? AND hour < ?`+sc, args...).Scan(&t.IP)
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(cnt),0) FROM dim WHERE dim = 'spider' AND hour >= ? AND hour < ?`+sc, args...).Scan(&t.Spiders)
	return t, nil
}

type TrendPoint struct {
	Time   string `json:"time"`
	PV     int64  `json:"pv"`
	Req    int64  `json:"req"`
	Bytes  int64  `json:"bytes"`
	Err4   int64  `json:"err4"`
	Err5   int64  `json:"err5"`
	UV     int64  `json:"uv"`
}

type Overview struct {
	Current  Totals       `json:"current"`
	Previous Totals       `json:"previous"`
	Trend    []TrendPoint `json:"trend"`
}

func (s *Store) Overview(req RangeReq) (*Overview, error) {
	start, end, pstart, byDay := window(req.Range, req.Start, req.End)
	cur, err := s.totals(req.Site, start, end)
	if err != nil {
		return nil, err
	}
	prev, err := s.totals(req.Site, pstart, start)
	if err != nil {
		return nil, err
	}
	sc, sa := siteClause(req.Site)
	args := append([]interface{}{start / 3600, (end + 3599) / 3600}, sa...)
	rows, err := s.db.Query(`SELECT hour, SUM(pv), SUM(req), SUM(bytes), SUM(e4), SUM(e5) FROM agg WHERE hour >= ? AND hour < ?`+sc+` GROUP BY hour`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	buckets := map[string]*TrendPoint{}
	for rows.Next() {
		var hour int64
		var p TrendPoint
		if err := rows.Scan(&hour, &p.PV, &p.Req, &p.Bytes, &p.Err4, &p.Err5); err != nil {
			return nil, err
		}
		t := time.Unix(hour*3600, 0)
		key := t.Format("15:00")
		if byDay {
			key = t.Format("01-02")
		}
		b := buckets[key]
		if b == nil {
			b = &TrendPoint{Time: key}
			buckets[key] = b
		}
		b.PV += p.PV
		b.Req += p.Req
		b.Bytes += p.Bytes
		b.Err4 += p.Err4
		b.Err5 += p.Err5
	}
	uvRows, err := s.db.Query(`SELECT h, COUNT(*) FROM vis2 WHERE hour >= ? AND hour < ?`+sc+` GROUP BY h`, args...)
	if err != nil {
		return nil, err
	}
	defer uvRows.Close()
	for uvRows.Next() {
		var h, n int64
		if err := uvRows.Scan(&h, &n); err != nil {
			return nil, err
		}
		t := time.Unix(h*3600, 0)
		key := t.Format("15:00")
		if byDay {
			key = t.Format("01-02")
		}
		b := buckets[key]
		if b == nil {
			b = &TrendPoint{Time: key}
			buckets[key] = b
		}
		b.UV += n
	}
	// emit every bucket of the window so charts have a continuous axis
	var trend []TrendPoint
	if byDay {
		for t := time.Unix(start, 0); t.Unix() < end; t = t.AddDate(0, 0, 1) {
			key := t.Format("01-02")
			if b := buckets[key]; b != nil {
				trend = append(trend, *b)
			} else {
				trend = append(trend, TrendPoint{Time: key})
			}
		}
	} else {
		for h := 0; h < 24; h++ {
			key := fmt.Sprintf("%02d:00", h)
			if b := buckets[key]; b != nil {
				trend = append(trend, *b)
			} else {
				trend = append(trend, TrendPoint{Time: key})
			}
		}
	}
	return &Overview{Current: cur, Previous: prev, Trend: trend}, rows.Err()
}

type DistReq struct {
	Site  string `json:"site"`
	Range string `json:"range"`
	Dim   string `json:"dim"`
	Limit int    `json:"limit"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

type DistItem struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

var allowedDims = map[string]bool{
	"country": true, "province": true, "status": true, "referer": true, "url": true,
	"ip": true, "browser": true, "os": true, "device": true, "spider": true,
}

func (s *Store) Distribution(req DistReq) ([]DistItem, error) {
	if !allowedDims[req.Dim] {
		return nil, fmt.Errorf("unknown dimension %s", req.Dim)
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 10
	}
	start, end, _, _ := window(req.Range, req.Start, req.End)
	sc, sa := siteClause(req.Site)
	args := append([]interface{}{req.Dim, start / 3600, (end + 3599) / 3600}, sa...)
	args = append(args, req.Limit)
	rows, err := s.db.Query(`SELECT k, SUM(cnt) c FROM dim WHERE dim = ? AND hour >= ? AND hour < ?`+sc+` GROUP BY k ORDER BY c DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DistItem{}
	for rows.Next() {
		var it DistItem
		if err := rows.Scan(&it.Name, &it.Count); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

type ErrReq struct {
	Site  string `json:"site"`
	Range string `json:"range"`
	Class string `json:"class"` // "4xx", "5xx" or empty
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

type ErrItem struct {
	Status int    `json:"status"`
	URL    string `json:"url"`
	Count  int64  `json:"count"`
	Last   int64  `json:"last"`
}

func (s *Store) Errors(req ErrReq) (map[string]interface{}, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 20
	}
	start, end, _, _ := window(req.Range, req.Start, req.End)
	sc, sa := siteClause(req.Site)
	args := append([]interface{}{start / 3600, (end + 3599) / 3600}, sa...)
	rows, err := s.db.Query(`SELECT k, SUM(cnt), MAX(last) FROM dim WHERE dim = 'error' AND hour >= ? AND hour < ?`+sc+` GROUP BY k`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []ErrItem
	for rows.Next() {
		var k string
		var it ErrItem
		if err := rows.Scan(&k, &it.Count, &it.Last); err != nil {
			return nil, err
		}
		parts := strings.SplitN(k, " ", 2)
		fmt.Sscanf(parts[0], "%d", &it.Status)
		if len(parts) > 1 {
			it.URL = parts[1]
		}
		if req.Class == "4xx" && it.Status >= 500 || req.Class == "5xx" && it.Status < 500 {
			continue
		}
		all = append(all, it)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Count > all[j].Count })
	total := len(all)
	from := (req.Page - 1) * req.Size
	if from > total {
		from = total
	}
	to := from + req.Size
	if to > total {
		to = total
	}
	return map[string]interface{}{"items": all[from:to], "total": total}, rows.Err()
}

type RealtimeReq struct {
	Site   string `json:"site"`
	IP     string `json:"ip"`
	URL    string `json:"url"`
	Region string `json:"region"`
	Method string `json:"method"`
	Status string `json:"status"` // "", "2xx".."5xx" or an exact code
	Spider string `json:"spider"` // "", "yes" or "no"
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Page   int    `json:"page"`
	Size   int    `json:"size"`
}

type Hit struct {
	ID      int64   `json:"id"`
	Time    int64   `json:"time"`
	Site    string  `json:"site"`
	IP      string  `json:"ip"`
	Region  string  `json:"region"`
	Method  string  `json:"method"`
	URL     string  `json:"url"`
	Status  int     `json:"status"`
	Bytes   int64   `json:"bytes"`
	Rt      float64 `json:"rt"`
	UA      string  `json:"ua"`
	Referer string  `json:"referer"`
}

// spiderCond is the SQL form of classifyUA's spider detection.
func spiderCond() string {
	var parts []string
	for _, sp := range spiders {
		parts = append(parts, "LOWER(ua) LIKE '%"+sp.key+"%'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

type LogStats struct {
	Req     int64   `json:"req"`
	OK      int64   `json:"ok"`
	Err     int64   `json:"err"`
	IPs     int64   `json:"ips"`
	Spiders int64   `json:"spiders"`
	AvgRt   float64 `json:"avgRt"` // seconds
	Bytes   int64   `json:"bytes"`
}

func (s *Store) Realtime(req RealtimeReq) (map[string]interface{}, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 20
	}
	if req.Size > 5000 {
		req.Size = 5000
	}
	where := " WHERE 1=1"
	var args []interface{}
	if req.Site != "" {
		where += " AND site = ?"
		args = append(args, req.Site)
	}
	if req.IP != "" {
		where += " AND ip LIKE ?"
		args = append(args, "%"+req.IP+"%")
	}
	if req.URL != "" {
		where += " AND url LIKE ?"
		args = append(args, "%"+req.URL+"%")
	}
	if req.Region != "" {
		where += " AND (country || ' ' || province) LIKE ?"
		args = append(args, "%"+req.Region+"%")
	}
	if req.Method != "" {
		where += " AND method = ?"
		args = append(args, strings.ToUpper(req.Method))
	}
	if req.Start > 0 && req.End > req.Start {
		where += " AND ts >= ? AND ts < ?"
		args = append(args, req.Start, req.End)
	}
	switch req.Status {
	case "":
	case "2xx", "3xx", "4xx", "5xx":
		lo := int(req.Status[0]-'0') * 100
		where += " AND status >= ? AND status < ?"
		args = append(args, lo, lo+100)
	default:
		where += " AND status = ?"
		args = append(args, req.Status)
	}
	switch req.Spider {
	case "yes":
		where += " AND " + spiderCond()
	case "no":
		where += " AND NOT " + spiderCond()
	}
	var st LogStats
	var rtsum float64
	var rtcnt int64
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(status = 200),0), COALESCE(SUM(status >= 400),0), COUNT(DISTINCT ip),
		COALESCE(SUM(`+spiderCond()+`),0), COALESCE(SUM(rt),0), COALESCE(SUM(rt > 0),0), COALESCE(SUM(bytes),0) FROM hits`+where, args...).
		Scan(&st.Req, &st.OK, &st.Err, &st.IPs, &st.Spiders, &rtsum, &rtcnt, &st.Bytes); err != nil {
		return nil, err
	}
	if rtcnt > 0 {
		st.AvgRt = rtsum / float64(rtcnt)
	}
	rows, err := s.db.Query(`SELECT id, ts, site, ip, country, province, method, url, status, bytes, rt, ua, referer FROM hits`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, req.Size, (req.Page-1)*req.Size)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Hit{}
	for rows.Next() {
		var h Hit
		var country, prov string
		if err := rows.Scan(&h.ID, &h.Time, &h.Site, &h.IP, &country, &prov, &h.Method, &h.URL, &h.Status, &h.Bytes, &h.Rt, &h.UA, &h.Referer); err != nil {
			return nil, err
		}
		h.Region = strings.TrimSpace(country + " " + prov)
		items = append(items, h)
	}
	return map[string]interface{}{"items": items, "total": st.Req, "stats": st}, rows.Err()
}

type LivePoint struct {
	Time  int64 `json:"time"`
	Req   int64 `json:"req"`
	Bytes int64 `json:"bytes"`
}

// Live reports the last minute of requests in 5 second slots.
func (s *Store) Live(site string) (map[string]interface{}, error) {
	now := time.Now().Unix()
	from := now - 60
	q := `SELECT (ts - ?) / 5, COUNT(*), COALESCE(SUM(bytes),0) FROM hits WHERE ts > ?`
	args := []interface{}{from, from}
	if site != "" {
		q += " AND site = ?"
		args = append(args, site)
	}
	rows, err := s.db.Query(q+" GROUP BY 1", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	series := make([]LivePoint, 12)
	for i := range series {
		series[i].Time = from + int64(i)*5 + 5
	}
	var req, bytes int64
	for rows.Next() {
		var slot, n, b int64
		if err := rows.Scan(&slot, &n, &b); err != nil {
			return nil, err
		}
		if slot >= 0 && slot < 12 {
			series[slot].Req, series[slot].Bytes = n, b
		}
		req += n
		bytes += b
	}
	return map[string]interface{}{"req": req, "bytes": bytes, "series": series}, rows.Err()
}

// SiteTotals returns the totals of every given website in one window.
func (s *Store) SiteTotals(aliases []string, req RangeReq) (map[string]Totals, error) {
	start, end, _, _ := window(req.Range, req.Start, req.End)
	res := map[string]Totals{}
	for _, a := range aliases {
		t, err := s.totals(a, start, end)
		if err != nil {
			return nil, err
		}
		res[a] = t
	}
	return res, nil
}
