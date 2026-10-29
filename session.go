// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// session is what is known of the call and the caller, handed to the
// selection in its environment, its command line and its session file
type session struct {
	User      string    `json:"user"`
	Password  string    `json:"password,omitempty"`
	Auth      string    `json:"auth"`
	Dialed    string    `json:"dialed"`
	CallerID  string    `json:"callerid"`
	Baud      string    `json:"baud"`
	Rate      string    `json:"rate"`
	TxRate    string    `json:"tx_rate"`
	RxRate    string    `json:"rx_rate"`
	Channel   string    `json:"channel"`
	Profile   string    `json:"profile"`
	Connected time.Time `json:"connected"`
	Selection string    `json:"selection"`
	TimeLeft  string    `json:"time_left"` // whole minutes; "": no limit
	// the same in seconds
	SecondsLeft string `json:"seconds_left"`
	// the caller's terminal, as detected ("": not asked): ansi, vt (an
	// ANSI terminal that said what it is), vt52, petscii, ascii
	Terminal string `json:"terminal"`
	Term     string `json:"term"`      // a TERM for it: xterm, vt220, ansi, vt52, dumb...
	TermName string `json:"term_name"` // what it said it is: xterm(390), VT220, SyncTERM (CTerm 1.316)
	Rows     string `json:"rows"`
	Cols     string `json:"cols"`
	// an upstream menu's deadline, from its session header
	upstreamDeadline time.Time

	groupCache map[string]bool // the backends' group answers, for the call
}

// newSession from app_dmodem's environment (DMODEM_RATE "tx/rx",
// DMODEM_CALLERID, DMODEM_DIALED, DMODEM_CHANNEL, DMODEM_PROFILE), what the
// command line overrides of it
func newSession(rate, dialed, callerID string, now time.Time) *session {
	s := &session{
		Dialed:    dialed,
		CallerID:  callerID,
		Channel:   os.Getenv("DMODEM_CHANNEL"),
		Profile:   os.Getenv("DMODEM_PROFILE"),
		Connected: now,
	}
	// what an upstream menu found the terminal to be
	s.Terminal = os.Getenv("MODEMMENU_UPSTREAM_TERMINAL")
	s.Term = os.Getenv("MODEMMENU_UPSTREAM_TERM")
	s.TermName = os.Getenv("MODEMMENU_UPSTREAM_TERM_NAME")
	s.Rows = os.Getenv("MODEMMENU_UPSTREAM_ROWS")
	s.Cols = os.Getenv("MODEMMENU_UPSTREAM_COLS")
	for _, k := range []string{"TERMINAL", "TERM", "TERM_NAME", "ROWS", "COLS"} {
		os.Unsetenv("MODEMMENU_UPSTREAM_" + k)
	}
	s.Rate = rate
	tx, rx, ok := strings.Cut(rate, "/")
	if !ok {
		rx = tx
	}
	if _, err := strconv.Atoi(tx); err == nil {
		s.TxRate, s.RxRate = tx, rx
		// the speed the caller's modem reports: what reaches it
		s.Baud = tx
	}
	return s
}

// Speed: the connect rate for the caller to read: "33600 bit/s", or with
// the directions apart "50666 bit/s down, 31200 bit/s up" (down: to the
// caller); "" when it is not known
func (s *session) Speed() string {
	tx, errT := strconv.Atoi(s.TxRate)
	rx, errR := strconv.Atoi(s.RxRate)
	switch {
	case errT != nil || tx <= 0:
		return ""
	case errR != nil || rx <= 0 || rx == tx:
		return strconv.Itoa(tx) + " bit/s"
	}
	return fmt.Sprintf("%d bit/s down, %d bit/s up", tx, rx)
}

// vars: the placeholders of templates and the environment, by name
func (s *session) vars(withPassword bool) map[string]string {
	v := map[string]string{
		"user":         s.User,
		"auth":         s.Auth,
		"dialed":       s.Dialed,
		"callerid":     s.CallerID,
		"baud":         s.Baud,
		"rate":         s.Rate,
		"tx_rate":      s.TxRate,
		"rx_rate":      s.RxRate,
		"speed":        s.Speed(),
		"channel":      s.Channel,
		"profile":      s.Profile,
		"selection":    s.Selection,
		"time_left":    s.TimeLeft,
		"seconds_left": s.SecondsLeft,
		"terminal":     s.Terminal,
		"term":         s.Term,
		"term_name":    s.TermName,
		"rows":         s.Rows,
		"cols":         s.Cols,
		"date":         s.Connected.Format("2006-01-02"),
		"time":         s.Connected.Format("15:04:05"),
		"datetime":     s.Connected.Format(time.RFC3339),
		"epoch":        strconv.FormatInt(s.Connected.Unix(), 10),
		"password":     "",
	}
	if withPassword {
		v["password"] = s.Password
	}
	return v
}

// environ: MODEM_<NAME> for every placeholder
func (s *session) environ(withPassword bool) []string {
	v := s.vars(withPassword)
	names := make([]string, 0, len(v))
	for k := range v {
		names = append(names, k)
	}
	sort.Strings(names)
	env := make([]string, 0, len(v))
	for _, k := range names {
		if k == "password" && !withPassword {
			continue
		}
		env = append(env, "MODEM_"+strings.ToUpper(k)+"="+v[k])
	}
	return env
}

// writeFile: the session as JSON in a file only its owner reads
func (s *session) writeFile(withPassword bool, uid, gid int) (string, error) {
	c := *s
	if !withPassword {
		c.Password = ""
	}
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "modemmenu-*.json")
	if err != nil {
		return "", err
	}
	if _, err = f.Write(append(b, '\n')); err == nil && uid >= 0 {
		err = f.Chown(uid, gid)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// expand replaces {name} by its value, quote() of it (nil: as is);
// unknown names stay as they are
func expand(t string, v map[string]string, quote func(string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(t, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(t[i:], '}')
		if j < 0 {
			break
		}
		name := t[i+1 : i+j]
		val, ok := v[strings.ToLower(name)]
		b.WriteString(t[:i])
		switch {
		case !ok:
			b.WriteString(t[i : i+j+1])
		case quote != nil:
			b.WriteString(quote(val))
		default:
			b.WriteString(val)
		}
		t = t[i+j+1:]
	}
	b.WriteString(t)
	return b.String()
}

// shellQuote: a value as one word of /bin/sh
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
