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

// TestSpeed: {speed} from the connect rates
func TestSpeed(t *testing.T) {
	for rate, want := range map[string]string{
		"33600/33600": "33600 bit/s",
		"33600/31200": "33600 bit/s down, 31200 bit/s up",
		"50666/31200": "50666 bit/s down, 31200 bit/s up",
		"2400":        "2400 bit/s",
		"":            "",
		"0/0":         "",
		"fast":        "",
	} {
		s := newSession(rate, "", "", time.Now())
		if got := s.Speed(); got != want {
			t.Errorf("%q: %q, want %q", rate, got, want)
		}
		if got := s.vars(false)["speed"]; got != want {
			t.Errorf("%q: {speed} %q, want %q", rate, got, want)
		}
		if env := strings.Join(s.environ(false), "\n"); !strings.Contains(env, "MODEM_SPEED="+want+"\n") {
			t.Errorf("%q: no MODEM_SPEED=%s in\n%s", rate, want, env)
		}
	}
}

// the connect rate after the banner, before the login prompt
func TestConnectMessage(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	conf := func(extra string) string {
		p := filepath.Join(dir, fmt.Sprintf("c%d.conf", len(extra)))
		os.WriteFile(p, []byte(fmt.Sprintf("[general]\nlog = none\nbanner = \"Test BBS\\r\\n\"\n%s\n[file]\npath = %s\n[item i]\ncommand = true\n", extra, users)), 0o600)
		return p
	}
	for _, c := range []struct {
		conf, rate, want string
	}{
		{conf(""), "DMODEM_RATE=33600/33600", "Test BBS\r\n\r\nConnected at 33600 bit/s.\r\n\r\nlogin: "},
		{conf(""), "DMODEM_RATE=50666/31200", "Connected at 50666 bit/s down, 31200 bit/s up.\r\n"},
		{conf(""), "DMODEM_RATE=", "Test BBS\r\n\r\nlogin: "},
		{conf(`connect_message = "CONNECT {tx_rate}\r\n"`), "DMODEM_RATE=26400/24000", "Test BBS\r\nCONNECT 26400\r\n\r\nlogin: "},
		{conf(`connect_message = ""`), "DMODEM_RATE=33600/33600", "Test BBS\r\n\r\nlogin: "},
	} {
		cl, cmd := callAs(t, bin, c.conf, c.rate)
		got := cl.expect("login: ")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q in it", c.rate, got, c.want)
		}
		cmd.Process.Kill()
	}
}
