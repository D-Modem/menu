// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCallersList: the list keeps the newest callersKept logins,
// newest first
func TestCallersList(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC)
	for i := 0; i < callersKept+5; i++ {
		s := &session{User: fmt.Sprintf("u%d", i), Baud: "33600"}
		if i == 3 {
			s.User = "tab\tand\nline"
		}
		if err := recordCaller(dir, s, start.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(callersPath(dir))
	if n := strings.Count(string(b), "\n"); n != callersKept {
		t.Errorf("%d kept, want %d", n, callersKept)
	}
	last, err := lastCallers(dir, 3)
	if err != nil || len(last) != 3 || last[0][1] != fmt.Sprintf("u%d", callersKept+4) || last[2][1] != fmt.Sprintf("u%d", callersKept+2) {
		t.Errorf("last %v %v", last, err)
	}
	if strings.Contains(string(b), "tab\tand") {
		t.Error("a tab in a name made it into the list")
	}
	if l, err := lastCallers(t.TempDir(), 5); err != nil || len(l) != 0 {
		t.Errorf("no list yet: %v %v", l, err)
	}
}

// TestCallersLine: on a line, a mode = callers item shows who called
func TestCallersLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\nbob:{PLAIN}secret\n"), 0o600)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
state_dir = %s
[file]
path = %s
[item callers]
key = LC
mode = callers
count = 5
`, filepath.Join(dir, "state"), users)), 0o600)
	for _, u := range []string{"alice", "bob"} {
		c, cmd := callAs(t, bin, conf, "DMODEM_RATE=26400/26400", "DMODEM_CALLERID=5559999")
		c.expect("login: ")
		c.send(u + "\r")
		c.expect("Password: ")
		c.send("secret\r")
		c.expect("Selection: ")
		c.send("lc\r")
		got := c.expect("Selection: ")
		if u == "bob" {
			bi, ai := strings.Index(got, "bob "), strings.Index(got, "alice ")
			if bi < 0 || ai < 0 || bi > ai || !strings.Contains(got, "26400 bit/s") || strings.Contains(got, "5559999") {
				t.Errorf("bob's list:\n%s", got)
			}
		}
		c.send("g\r")
		waitExit(t, cmd)
	}
}
