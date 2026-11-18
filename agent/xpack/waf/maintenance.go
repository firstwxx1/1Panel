package waf

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func intFromAny(v interface{}, def int) int {
	switch n := v.(type) {
	case float64:
		if int(n) > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	case int64:
		if n > 0 {
			return int(n)
		}
	case json.Number:
		if i, err := n.Int64(); err == nil && i > 0 {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(n); err == nil && i > 0 {
			return i
		}
	}
	return def
}

func dbFilesTotalSize(paths ...string) int64 {
	var total int64
	for _, p := range paths {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if st, err := os.Stat(p + suffix); err == nil && !st.IsDir() {
				total += st.Size()
			}
		}
	}
	return total
}

func tableHasColumn(db *sql.DB, table, col string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info('%s')", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil && name == col {
			return true
		}
	}
	return false
}

func trimOldestPercent(db *sql.DB, table string, pct int) error {
	var total int64
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&total); err != nil {
		if isMissingDB(err) {
			return nil
		}
		return err
	}
	if total <= 0 {
		return nil
	}
	drop := (total * int64(pct)) / 100
	if drop < 1 {
		drop = 1
	}
	_, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id IN (SELECT id FROM %s ORDER BY id ASC LIMIT ?)", table, table), drop)
	if isMissingDB(err) {
		return nil
	}
	return err
}

// ClearLogs pauses engine log writes if active and clears both attack_logs.db and nginx_logs.db.
func (s *Store) ClearLogs() error {
	if s.isGlobalWafOn() {
		ownerID := fmt.Sprintf("clear-%d", time.Now().UnixNano())
		_, _ = s.enginePost("log_maintenance", map[string]interface{}{"action": "begin", "owner": ownerID})
		defer func() {
			_, _ = s.enginePost("log_maintenance", map[string]interface{}{"action": "finish", "owner": ownerID})
		}()
	}
	return ClearLogs(s.OpenrestyPath)
}

// RunMaintenance enforces log.maxDay and log.maxSize on attack_logs.db and nginx_logs.db.
func (s *Store) RunMaintenance() error {
	maxDay := 180
	maxSizeGB := 1
	var cfg map[string]interface{}
	if err := readJSON(s.globalFile(), &cfg); err == nil {
		if logCfg, ok := cfg["log"].(map[string]interface{}); ok {
			maxDay = intFromAny(logCfg["maxDay"], 180)
			maxSizeGB = intFromAny(logCfg["maxSize"], 1)
		}
	}

	if s.isGlobalWafOn() {
		ownerID := fmt.Sprintf("maint-%d", time.Now().UnixNano())
		_, _ = s.enginePost("log_maintenance", map[string]interface{}{"action": "begin", "owner": ownerID})
		defer func() {
			_, _ = s.enginePost("log_maintenance", map[string]interface{}{"action": "finish", "owner": ownerID})
		}()
	}

	cutoff := time.Now().AddDate(0, 0, -maxDay)
	cutoffStr := cutoff.Format("2006-01-02 15:04:05")
	cutoffUnix := cutoff.Unix()

	attackPath := dbFilePath(s.OpenrestyPath, "waf", "attack_logs")
	nginxPath := dbFilePath(s.OpenrestyPath, "waf", "nginx_logs")

	var minNginxLogID int64
	var remainingAttacks int64
	var attackDeleted int64

	if adb, err := openDBRW(s.OpenrestyPath, "waf", "attack_logs"); err == nil {
		if tableHasColumn(adb, "attack_logs", "localtime") {
			if res, err := adb.Exec(`DELETE FROM attack_logs WHERE localtime != '' AND localtime < ?`, cutoffStr); err == nil {
				attackDeleted, _ = res.RowsAffected()
			}
		} else if tableHasColumn(adb, "attack_logs", "create_date") {
			if res, err := adb.Exec(`DELETE FROM attack_logs WHERE create_date != '' AND create_date < ?`, cutoffStr); err == nil {
				attackDeleted, _ = res.RowsAffected()
			}
		} else if tableHasColumn(adb, "attack_logs", "time") {
			if res, err := adb.Exec(`DELETE FROM attack_logs WHERE time > 0 AND time < ?`, cutoffUnix); err == nil {
				attackDeleted, _ = res.RowsAffected()
			}
		}
		_ = adb.QueryRow(`SELECT COALESCE(MIN(nginx_log_id), 0), COUNT(*) FROM attack_logs WHERE nginx_log_id > 0`).Scan(&minNginxLogID, &remainingAttacks)
		_, _ = adb.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		adb.Close()
	} else if !isMissingDB(err) {
		return err
	}

	if ndb, err := openDBRW(s.OpenrestyPath, "waf", "nginx_logs"); err == nil {
		switch {
		case tableHasColumn(ndb, "nginx_logs", "localtime"):
			_, _ = ndb.Exec(`DELETE FROM nginx_logs WHERE localtime != '' AND localtime < ?`, cutoffStr)
		case tableHasColumn(ndb, "nginx_logs", "create_date"):
			_, _ = ndb.Exec(`DELETE FROM nginx_logs WHERE create_date != '' AND create_date < ?`, cutoffStr)
		case tableHasColumn(ndb, "nginx_logs", "time"):
			_, _ = ndb.Exec(`DELETE FROM nginx_logs WHERE time > 0 AND time < ?`, cutoffUnix)
		case remainingAttacks > 0 && minNginxLogID > 1:
			_, _ = ndb.Exec(`DELETE FROM nginx_logs WHERE id < ?`, minNginxLogID)
		case remainingAttacks == 0 && attackDeleted > 0:
			_, _ = ndb.Exec(`DELETE FROM nginx_logs`)
		}
		_, _ = ndb.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		ndb.Close()
	} else if !isMissingDB(err) {
		return err
	}

	maxBytes := int64(maxSizeGB) * 1024 * 1024 * 1024
	if dbFilesTotalSize(attackPath, nginxPath) > maxBytes {
		for _, target := range []struct{ dbName, table string }{
			{"attack_logs", "attack_logs"},
			{"nginx_logs", "nginx_logs"},
		} {
			db, err := openDBRW(s.OpenrestyPath, "waf", target.dbName)
			if err != nil {
				if isMissingDB(err) {
					continue
				}
				return err
			}
			_ = trimOldestPercent(db, target.table, 20)
			_, _ = db.Exec("VACUUM")
			_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
			db.Close()
		}
	}
	return nil
}
