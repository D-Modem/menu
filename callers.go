// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// The last callers: each login as a line of state_dir/callers (the time,
// the user, the rate; not the caller ID, which other callers would see),
// the latest callersKept of them kept, for mode = callers items.

const callersKept = 200

// callersPath: the last callers' file in state_dir
func callersPath(dir string) string { return filepath.Join(dir, "callers") }

// recordCaller: sess's login into the list, under a lock (calls at once)
func recordCaller(dir string, sess *session, now time.Time) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(callersPath(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	b := make([]byte, st.Size())
	f.ReadAt(b, 0)
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, s)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if lines[0] == "" {
		lines = nil
	}
	lines = append(lines, fmt.Sprintf("%s\t%s\t%s", now.Format(time.RFC3339), clean(sess.User), clean(sess.Baud)))
	if len(lines) > callersKept {
		lines = lines[len(lines)-callersKept:]
	}
	out := []byte(strings.Join(lines, "\n") + "\n")
	if _, err := f.WriteAt(out, 0); err != nil {
		return err
	}
	return f.Truncate(int64(len(out)))
}

// lastCallers: the latest n, the newest first
func lastCallers(dir string, n int) ([][3]string, error) {
	b, err := os.ReadFile(callersPath(dir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	var out [][3]string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		p := strings.SplitN(lines[i], "\t", 3)
		if len(p) == 3 {
			out = append(out, [3]string{p[0], p[1], p[2]})
		}
	}
	return out, nil
}

// callersItem: mode = callers, the last count callers on the line
func callersItem(t *term, s *settings, it *menuItem) error {
	list, err := lastCallers(s.limits.stateDir, it.count)
	if err != nil {
		logf("callers: %v", err)
	}
	var b strings.Builder
	b.WriteString("\r\nLast callers\r\n\r\n")
	if len(list) == 0 {
		b.WriteString("  (none yet)\r\n")
	}
	for _, c := range list {
		when := c[0]
		if tm, err := time.Parse(time.RFC3339, c[0]); err == nil {
			when = tm.Local().Format("2006-01-02 15:04")
		}
		rate := c[2]
		if rate != "" {
			rate += " bit/s"
		}
		fmt.Fprintf(&b, "  %s  %-16s %s\r\n", when, c[1], rate)
	}
	return t.write(b.String())
}

// hasCallers: whether an item shows the last callers (logins are only
// recorded then)
func (s *settings) hasCallers() bool {
	for _, it := range s.items {
		if it.mode == "callers" {
			return true
		}
	}
	return false
}
