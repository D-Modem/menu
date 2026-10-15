// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// errTimeLimit: the session's time is up
var errTimeLimit = errors.New("time limit")

// A user's use of the day is kept in state_dir/usage/USER: the date and
// the seconds used, "2026-10-12 1830".  Concurrent calls of a user (or
// sessions of modemmenu -serve) add to it under a lock.

func usagePath(dir, user string) string {
	return filepath.Join(dir, "usage", url.PathEscape(user))
}

// usedToday: what user has used of day (YYYY-MM-DD)
func usedToday(dir, user, day string) (time.Duration, error) {
	f, err := os.Open(usagePath(dir, user))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	unix.Flock(int(f.Fd()), unix.LOCK_SH)
	b := make([]byte, 64)
	n, _ := f.Read(b)
	return parseUsage(string(b[:n]), day), nil
}

func parseUsage(s, day string) time.Duration {
	d, secs, ok := strings.Cut(strings.TrimSpace(s), " ")
	if !ok || d != day {
		return 0
	}
	n, err := strconv.ParseInt(secs, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// addUsage: used more of day for user
func addUsage(dir, user, day string, used time.Duration) error {
	p := usagePath(dir, user)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	b := make([]byte, 64)
	n, _ := f.ReadAt(b, 0)
	total := parseUsage(string(b[:n]), day) + used
	line := fmt.Sprintf("%s %d\n", day, int64(total/time.Second))
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.WriteAt([]byte(line), 0)
	return err
}

// limiter: the time limits of a logged-in session
type limiter struct {
	s        *settings
	sess     *session
	start    time.Time
	day      string
	deadline time.Time // zero: none
	record   sync.Once
}

// newLimiter at the login: the session's deadline, from time_limit (the
// guest's for a guest) and what is left of daily_limit; errTimeLimit when
// nothing is left today
func newLimiter(s *settings, sess *session, now time.Time) (*limiter, error) {
	l := &limiter{s: s, sess: sess, start: now, day: now.Format("2006-01-02")}
	limit := s.limits.timeLimit
	if sess.Auth == "guest" {
		limit = s.guest.timeLimit
	}
	if limit > 0 {
		l.deadline = now.Add(limit)
	}
	if l.daily() {
		used, err := usedToday(s.limits.stateDir, sess.User, l.day)
		if err != nil {
			// no record: no limit is better than no login
			logf("daily_limit: %v", err)
			return l, nil
		}
		left := s.limits.dailyLimit - used
		if left <= 0 {
			return l, errTimeLimit
		}
		if d := now.Add(left); l.deadline.IsZero() || d.Before(l.deadline) {
			l.deadline = d
		}
	}
	return l, nil
}

// daily: whether the session counts towards a daily limit (guests share
// one name: they have time_limit only)
func (l *limiter) daily() bool {
	return l.s.limits.dailyLimit > 0 && l.sess.Auth != "guest"
}

// left: the time left, and whether there is a limit at all
func (l *limiter) left(now time.Time) (time.Duration, bool) {
	if l.deadline.IsZero() {
		return 0, false
	}
	return l.deadline.Sub(now), true
}

// timeLeft for {time_left}: whole minutes, "" without a limit
func (l *limiter) timeLeft(now time.Time) string {
	left, ok := l.left(now)
	if !ok {
		return ""
	}
	if left < 0 {
		left = 0
	}
	return strconv.Itoa(int(left / time.Minute))
}

// done: the session's time into the day's use, once (the session's end,
// or the hangup signal, whichever comes first)
func (l *limiter) done() {
	l.record.Do(func() {
		if !l.daily() {
			return
		}
		if err := addUsage(l.s.limits.stateDir, l.sess.User, l.day, time.Since(l.start)); err != nil {
			logf("daily_limit: %v", err)
		}
	})
}

// atExit: what the hangup signal does before the process ends
var (
	atExitMu sync.Mutex
	atExit   func()
)

func setAtExit(f func()) {
	atExitMu.Lock()
	atExit = f
	atExitMu.Unlock()
}

func runAtExit() {
	atExitMu.Lock()
	f := atExit
	atExitMu.Unlock()
	if f != nil {
		f()
	}
}
