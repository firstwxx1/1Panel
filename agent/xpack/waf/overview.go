package waf

import "time"

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
	todayStr := time.Now().Format("2006-01-02")
	res := &Overview{
		Today: DayStat{Day: todayStr},
		Trend: []DayStat{},
	}
	db, err := openDB(openrestyPath, "waf", "waf_stat")
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT day, req_count, attack_count, count4xx, count5xx FROM waf_stat ORDER BY id DESC LIMIT 7`)
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s DayStat
		if err := rows.Scan(&s.Day, &s.ReqCount, &s.AttackCount, &s.Count4xx, &s.Count5xx); err != nil {
			return nil, err
		}
		res.Trend = append([]DayStat{s}, res.Trend...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if n := len(res.Trend); n > 0 && res.Trend[n-1].Day == todayStr {
		res.Today = res.Trend[n-1]
	} else {
		res.Today = DayStat{Day: todayStr}
	}
	return res, nil
}
