package waf

import (
	"encoding/csv"
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
	if q.Size < 1 {
		q.Size = 20
	}
	if q.Size > 10000 {
		q.Size = 10000
	}

	d := newDBs(s.OpenrestyPath)
	defer d.close()
	bdb, err := d.get("waf", "block_ips")
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return res, err
	}

	where := "1=1"
	var args []interface{}
	if needle := strings.TrimSpace(q.IP); needle != "" {
		matchIDs, err := d.findIDs("waf", "ips", "ips", needle, true)
		if err != nil {
			return res, err
		}
		if len(matchIDs) == 0 {
			return res, nil
		}
		where = "ip_id IN (" + placeholders(len(matchIDs)) + ")"
		args = append(args, matchIDs...)
	}

	if err := bdb.QueryRow("SELECT COUNT(*) FROM block_ips WHERE "+where, args...).Scan(&res.Total); err != nil {
		if isMissingDB(err) {
			return res, nil
		}
		return res, err
	}
	if res.Total == 0 {
		return res, nil
	}

	queryArgs := append(append([]interface{}{}, args...), q.Size, (q.Page-1)*q.Size)
	rows, err := bdb.Query(`SELECT id, ip_id, attack_log_id, COALESCE(create_date,''), blocking_time FROM block_ips WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		if isMissingDB(err) {
			return res, nil
		}
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
	if len(list) == 0 {
		return res, nil
	}

	active := map[string]bool{}
	if ips, err := s.EngineBlockedIPs(); err == nil {
		for _, ip := range ips {
			active[ip] = true
		}
	}

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
		for _, chunk := range idChunks(logIDs) {
			if arows, err := adb.Query("SELECT id, rule_type_id, ip_location_id FROM attack_logs WHERE id IN ("+placeholders(len(chunk))+")", chunk...); err == nil {
				for arows.Next() {
					var id, rt, loc int
					if arows.Scan(&id, &rt, &loc) == nil {
						ruleOf[id], locOf[id] = rt, loc
					}
				}
				arows.Close()
			}
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
	for _, b := range list {
		item := BlockItem{
			ID: b.id, IP: ips[b.ipID], Rule: types[ruleOf[b.logID]], BlockedAt: b.at, BlockingSec: b.sec,
			Country: locs[locOf[b.logID]].Country, Province: locs[locOf[b.logID]].Province,
			Status: BlockStatusReleased,
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", b.at, time.Local); err == nil {
			left := b.sec - int(now.Sub(t).Seconds())
			if left > 0 && active[item.IP] {
				item.Status, item.Remaining = BlockStatusActive, left
			}
		}
		res.Items = append(res.Items, item)
	}
	return res, nil
}

// RemoveBlocks releases the given IPs in the engine and drops their block records.
func RemoveBlocks(s *Store, ips []string) error {
	if len(ips) == 0 {
		return nil
	}
	if err := s.EngineUnblockBatch(ips); err != nil {
		return err
	}
	return deleteBlockRows(s, ips)
}

// ClearBlocks releases every blocked IP and empties the block table.
func ClearBlocks(s *Store) error {
	if ips, err := s.EngineBlockedIPs(); err == nil && len(ips) > 0 {
		_ = s.EngineUnblockBatch(ips)
	}
	db, err := openDBRW(s.OpenrestyPath, "waf", "block_ips")
	if err != nil {
		if isMissingDB(err) {
			return nil
		}
		return err
	}
	defer db.Close()
	if _, err = db.Exec("DELETE FROM block_ips"); err != nil && !isMissingDB(err) {
		return err
	}
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return nil
}

// deleteBlockRows removes the block records of the given IPs.
func deleteBlockRows(s *Store, ips []string) error {
	d := newDBs(s.OpenrestyPath)
	defer d.close()
	ipdb, err := d.get("waf", "ips")
	if err != nil {
		if isMissingDB(err) {
			return nil
		}
		return err
	}
	var ids []interface{}
	for i := 0; i < len(ips); i += maxInBatch {
		end := i + maxInBatch
		if end > len(ips) {
			end = len(ips)
		}
		chunk := ips[i:end]
		args := make([]interface{}, 0, len(chunk))
		for _, ip := range chunk {
			args = append(args, ip)
		}
		rows, err := ipdb.Query("SELECT id FROM ips WHERE value IN ("+placeholders(len(args))+")", args...)
		if err != nil {
			if isMissingDB(err) {
				return nil
			}
			return err
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	if len(ids) == 0 {
		return nil
	}
	db, err := openDBRW(s.OpenrestyPath, "waf", "block_ips")
	if err != nil {
		if isMissingDB(err) {
			return nil
		}
		return err
	}
	defer db.Close()
	for i := 0; i < len(ids); i += maxInBatch {
		end := i + maxInBatch
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		if _, err = db.Exec("DELETE FROM block_ips WHERE ip_id IN ("+placeholders(len(chunk))+")", chunk...); err != nil && !isMissingDB(err) {
			return err
		}
	}
	return nil
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
