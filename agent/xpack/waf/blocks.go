package waf

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BlockItem is one row of the 封锁记录 table.
type BlockItem struct {
	ID          int    `json:"id"`
	IP          string `json:"ip"`
	Country     string `json:"country"`
	Province    string `json:"province"`
	Rule        string `json:"rule"`
	BlockedAt   string `json:"blockedAt"`
	BlockingSec int    `json:"blockingTime"`
	Remaining   int    `json:"remaining"`
	Status      string `json:"status"`
}

type BlockQuery struct {
	IP   string `json:"ip"`
	Page int    `json:"page"`
	Size int    `json:"size"`
}

type BlockPage struct {
	Total int         `json:"total"`
	Items []BlockItem `json:"items"`
}

const (
	BlockStatusActive   = "blocking"
	BlockStatusReleased = "released"
)

// ListBlocks returns the block events recorded by the engine. A row is reported as
// still blocking when the engine holds the IP in its shared dict and the duration
// has not elapsed; everything else is history.
func ListBlocks(s *Store, q BlockQuery) (*BlockPage, error) {
	res := &BlockPage{Items: []BlockItem{}}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 || q.Size > 200 {
		q.Size = 20
	}
	active := map[string]bool{}
	if ips, err := s.EngineBlockedIPs(); err == nil {
		for _, ip := range ips {
			active[ip] = true
		}
	}

	d := newDBs(s.OpenrestyPath)
	defer d.close()
	bdb, err := d.get("waf", "block_ips")
	if err != nil {
		return res, err
	}
	rows, err := bdb.Query(`SELECT id, ip_id, attack_log_id, COALESCE(create_date,''), blocking_time FROM block_ips ORDER BY id DESC LIMIT 2000`)
	if err != nil {
		return res, err
	}
	type brow struct {
		id, ipID, logID, sec int
		at                   string
	}
	var list []brow
	for rows.Next() {
		var b brow
		if err := rows.Scan(&b.id, &b.ipID, &b.logID, &b.at, &b.sec); err != nil {
			rows.Close()
			return res, err
		}
		list = append(list, b)
	}
	rows.Close()

	ipIDs, logIDs := map[int]bool{}, map[int]bool{}
	for _, b := range list {
		ipIDs[b.ipID] = true
		logIDs[b.logID] = true
	}
	ips, err := d.values("waf", "ips", "ips", ipIDs)
	if err != nil {
		return res, err
	}
	ruleOf, locOf := map[int]int{}, map[int]int{}
	if adb, err := d.get("waf", "attack_logs"); err == nil && len(logIDs) > 0 {
		holders, a := idList(logIDs)
		if arows, err := adb.Query("SELECT id, rule_type_id, ip_location_id FROM attack_logs WHERE id IN ("+holders+")", a...); err == nil {
			for arows.Next() {
				var id, rt, loc int
				if arows.Scan(&id, &rt, &loc) == nil {
					ruleOf[id], locOf[id] = rt, loc
				}
			}
			arows.Close()
		}
	}
	rtIDs, locIDs := map[int]bool{}, map[int]bool{}
	for _, v := range ruleOf {
		rtIDs[v] = true
	}
	for _, v := range locOf {
		locIDs[v] = true
	}
	types, _ := d.values("waf", "rule_types", "rule_types", rtIDs)
	locs, _ := d.locations(locIDs)

	now := time.Now()
	needle := strings.TrimSpace(q.IP)
	for _, b := range list {
		item := BlockItem{
			ID: b.id, IP: ips[b.ipID], Rule: types[ruleOf[b.logID]], BlockedAt: b.at, BlockingSec: b.sec,
			Country: locs[locOf[b.logID]].Country, Province: locs[locOf[b.logID]].Province,
			Status: BlockStatusReleased,
		}
		if needle != "" && !strings.Contains(item.IP, needle) {
			continue
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", b.at, time.Local); err == nil {
			left := b.sec - int(now.Sub(t).Seconds())
			if left > 0 && active[item.IP] {
				item.Status, item.Remaining = BlockStatusActive, left
			}
		}
		res.Items = append(res.Items, item)
	}
	res.Total = len(res.Items)
	start := (q.Page - 1) * q.Size
	if start > len(res.Items) {
		start = len(res.Items)
	}
	end := start + q.Size
	if end > len(res.Items) {
		end = len(res.Items)
	}
	res.Items = res.Items[start:end]
	return res, nil
}

// RemoveBlocks releases the given IPs in the engine and drops their block records.
func RemoveBlocks(s *Store, ips []string) error {
	if len(ips) == 0 {
		return nil
	}
	var failures []string
	for _, ip := range ips {
		if err := s.EngineUnblock(ip); err != nil {
			failures = append(failures, ip)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed to unblock: %s", strings.Join(failures, ","))
	}
	return deleteBlockRows(s, ips)
}

// ClearBlocks releases every blocked IP and empties the block table.
func ClearBlocks(s *Store) error {
	if ips, err := s.EngineBlockedIPs(); err == nil {
		for _, ip := range ips {
			_ = s.EngineUnblock(ip)
		}
	}
	db, err := openDBRW(s.OpenrestyPath, "waf", "block_ips")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("DELETE FROM block_ips")
	return err
}

// deleteBlockRows removes the block records of the given IPs.
func deleteBlockRows(s *Store, ips []string) error {
	d := newDBs(s.OpenrestyPath)
	defer d.close()
	ipdb, err := d.get("waf", "ips")
	if err != nil {
		return err
	}
	holders := strings.TrimSuffix(strings.Repeat("?,", len(ips)), ",")
	args := make([]interface{}, 0, len(ips))
	for _, ip := range ips {
		args = append(args, ip)
	}
	rows, err := ipdb.Query("SELECT id FROM ips WHERE value IN ("+holders+")", args...)
	if err != nil {
		return err
	}
	var ids []interface{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return nil
	}
	db, err := openDBRW(s.OpenrestyPath, "waf", "block_ips")
	if err != nil {
		return err
	}
	defer db.Close()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	_, err = db.Exec("DELETE FROM block_ips WHERE ip_id IN ("+ph+")", ids...)
	return err
}

// ExportBlocks renders the current view as CSV.
func ExportBlocks(items []BlockItem) ([]byte, error) {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	_ = w.Write([]string{"IP", "blocked_at", "status", "blocking_time", "remaining", "location", "rule"})
	for _, it := range items {
		_ = w.Write([]string{it.IP, it.BlockedAt, it.Status, strconv.Itoa(it.BlockingSec),
			strconv.Itoa(it.Remaining), strings.TrimSpace(it.Country + " " + it.Province), it.Rule})
	}
	w.Flush()
	return []byte(sb.String()), w.Error()
}
