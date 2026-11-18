package waf

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

var errDBNotInit = errors.New("waf database not initialized")

func isMissingDB(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errDBNotInit) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "unable to open database file")
}

// dataDir is the 1pwaf data directory of the installed OpenResty app.
func dataDir(openrestyPath string) string {
	return path.Join(openrestyPath, "1pwaf", "data")
}

func dbFilePath(openrestyPath, group, name string) string {
	return path.Join(dataDir(openrestyPath), "db", group, name+".db")
}

// openDB opens one of the sqlite files written by the 1pwaf engine, read-only.
func openDB(openrestyPath, group, name string) (*sql.DB, error) {
	file := dbFilePath(openrestyPath, group, name)
	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return nil, errDBNotInit
		}
		return nil, err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(3000)", file))
	if err != nil {
		return nil, err
	}
	return db, nil
}

// openDBRW opens an engine sqlite file for writing (clear / delete / maintenance operations).
func openDBRW(openrestyPath, group, name string) (*sql.DB, error) {
	file := dbFilePath(openrestyPath, group, name)
	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return nil, errDBNotInit
		}
		return nil, err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", file))
	if err != nil {
		return nil, err
	}
	return db, nil
}
