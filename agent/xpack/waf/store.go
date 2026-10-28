package waf

import (
	"database/sql"
	"fmt"
	"path"

	_ "github.com/glebarez/go-sqlite"
)

// dataDir is the 1pwaf data directory of the installed OpenResty app.
func dataDir(openrestyPath string) string {
	return path.Join(openrestyPath, "1pwaf", "data")
}

// openDB opens one of the sqlite files written by the 1pwaf engine, read-only.
func openDB(openrestyPath, group, name string) (*sql.DB, error) {
	file := path.Join(dataDir(openrestyPath), "db", group, name+".db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(3000)", file))
	if err != nil {
		return nil, err
	}
	return db, nil
}

// openDBRW opens an engine sqlite file for writing (clear / delete operations).
func openDBRW(openrestyPath, group, name string) (*sql.DB, error) {
	file := path.Join(dataDir(openrestyPath), "db", group, name+".db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", file))
	if err != nil {
		return nil, err
	}
	return db, nil
}
