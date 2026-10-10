package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// sqlAuth: query, with the user (and the password, for args = user
// password) as its arguments, returns the stored password of the user
// (checked as hash says), or with hash = none any row for a valid login
type sqlAuth struct {
	driver  string
	dsn     string
	query   string
	args    []string
	scheme  string
	timeout time.Duration
	db      *sql.DB
}

var sqlDrivers = map[string]string{
	"mysql":      "mysql",
	"mariadb":    "mysql",
	"postgres":   "postgres",
	"postgresql": "postgres",
	"sqlite":     "sqlite",
	"sqlite3":    "sqlite",
}

func newSQLAuth(s *section) (authenticator, error) {
	var err error
	a := &sqlAuth{
		dsn:    s.get("dsn", ""),
		query:  s.get("query", ""),
		args:   fields(strings.ToLower(s.get("args", "user"))),
		scheme: strings.ToLower(s.get("hash", "auto")),
	}
	drv := strings.ToLower(s.get("driver", ""))
	if a.driver = sqlDrivers[drv]; a.driver == "" {
		return nil, fmt.Errorf("[sql] driver: %q (mysql, postgres or sqlite)", drv)
	}
	if a.dsn == "" || a.query == "" {
		return nil, fmt.Errorf("[sql] needs dsn and query")
	}
	for _, arg := range a.args {
		if arg != "user" && arg != "password" {
			return nil, fmt.Errorf("[sql] args: %q (user, password)", arg)
		}
	}
	if a.timeout, err = s.duration("timeout", 10*time.Second); err != nil {
		return nil, err
	}
	// opening does not connect: a database that is down only fails the
	// logins that reach it
	if a.db, err = sql.Open(a.driver, a.dsn); err != nil {
		return nil, fmt.Errorf("[sql] %v", err)
	}
	a.db.SetMaxOpenConns(1)
	return a, nil
}

func (a *sqlAuth) name() string { return "sql" }

func (a *sqlAuth) authenticate(ctx context.Context, user, password string) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	args := make([]any, len(a.args))
	for i, arg := range a.args {
		if arg == "user" {
			args[i] = user
		} else {
			args[i] = password
		}
	}
	var stored sql.NullString
	row := a.db.QueryRowContext(ctx, a.query, args...)
	var err error
	if a.scheme == "none" {
		err = row.Scan(&discard{})
	} else {
		err = row.Scan(&stored)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errNoUser
	}
	if err != nil {
		return err
	}
	if a.scheme == "none" {
		return nil
	}
	if !stored.Valid {
		return errBadPassword
	}
	ok, err := checkPassword(stored.String, password, a.scheme)
	if err != nil {
		return fmt.Errorf("user %s: %v", user, err)
	}
	if !ok {
		return errBadPassword
	}
	return nil
}

// discard scans and drops a column of any type
type discard struct{}

func (*discard) Scan(any) error { return nil }
