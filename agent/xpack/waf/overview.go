package waf

type DayStat struct {
	Day         string `json:"day"`
	ReqCount    int64  `json:"reqCount"`
	AttackCount int64  `json:"attackCount"`
	Count4xx    int64  `json:"count4xx"`
	Count5xx    int64  `json:"count5xx"`
}

type Overview struct {
	Today DayStat   `json:"today"`
	Trend []DayStat `json:"trend"`
}

// LoadOverview returns the latest daily counters (up to 7 days) recorded by the engine.
func LoadOverview(openrestyPath string) (*Overview, error) {
	db, err := openDB(openrestyPath, "waf", "waf_stat")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT day, req_count, attack_count, count4xx, count5xx FROM waf_stat ORDER BY id DESC LIMIT 7`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := &Overview{Trend: []DayStat{}}
	for rows.Next() {
		var s DayStat
		if err := rows.Scan(&s.Day, &s.ReqCount, &s.AttackCount, &s.Count4xx, &s.Count5xx); err != nil {
			return nil, err
		}
		res.Trend = append([]DayStat{s}, res.Trend...)
	}
	if n := len(res.Trend); n > 0 {
		res.Today = res.Trend[n-1]
	}
	return res, nil
}
