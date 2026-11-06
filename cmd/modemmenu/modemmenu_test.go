// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/D-Modem/menu/internal/config"
)

// TestExpand: {placeholders} replaced and shell-quoted; unknown ones
// kept
func TestExpand(t *testing.T) {
	v := map[string]string{"user": "o'neil; rm -rf /", "baud": "33600"}
	got := config.Expand("login {user} {baud} {unknown} {", v, shellQuote)
	want := `login 'o'\''neil; rm -rf /' '33600' {unknown} {`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := config.Expand("{USER}/{baud}", v, nil); got != v["user"]+"/33600" {
		t.Errorf("got %q", got)
	}
	if got := config.Unescape(`a\r\n\x41\\b`); got != "a\r\nA\\b" {
		t.Errorf("unescape: %q", got)
	}
}

// TestSession: the call's details in the environment
func TestSession(t *testing.T) {
	now := time.Date(2026, 10, 10, 21, 5, 9, 0, time.UTC)
	s := newSession("50666/31200", "5551000", "5559999", now)
	s.User, s.Password = "alice", "secret"
	env := strings.Join(s.environ(true), "\n")
	for _, w := range []string{"MODEM_USER=alice", "MODEM_PASSWORD=secret", "MODEM_BAUD=50666",
		"MODEM_RX_RATE=31200", "MODEM_DIALED=5551000", "MODEM_CALLERID=5559999",
		"MODEM_DATE=2026-10-10", "MODEM_TIME=21:05:09", "MODEM_EPOCH=" + "1791666309"} {
		if !strings.Contains(env, w+"\n") && !strings.HasSuffix(env, w) {
			t.Errorf("no %s in\n%s", w, env)
		}
	}
	if strings.Contains(strings.Join(s.environ(false), "\n"), "secret") {
		t.Error("the password without pass_password")
	}
}

// TestMenuSettings: what keys, names and aliases select, and the menu
// settings that are refused
func TestMenuSettings(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	load := func(menu, items string) (*settings, error) {
		p := filepath.Join(dir, "c.conf")
		os.WriteFile(p, []byte("[general]\nauth = file\n[file]\npath = "+users+"\n[menu]\n"+menu+"\n"+items), 0o600)
		return loadSettings(p)
	}
	ppp := "[item ppp]\nkey = P\naliases = dialup net\ncommand = true\n"
	s, err := load("", ppp+"[item bbs]\nkey = BBS\ncommand = true\n")
	if err != nil {
		t.Fatal(err)
	}
	for choice, want := range map[string]string{"P": "ppp", "PPP": "ppp", "NET": "ppp", "BBS": "bbs", "B": ""} {
		it := s.selects(choice)
		if want == "" && it != nil || want != "" && (it == nil || it.name != want) {
			t.Errorf("%s selects %v, want %q", choice, it, want)
		}
	}
	if !contains(s.hangup, "GOODBYE") {
		t.Errorf("hangup %v", s.hangup)
	}
	for _, c := range []struct{ menu, items string }{
		{"", ppp + "[item net]\ncommand = true\n"}, // net: an alias of ppp
		{"hangup = P", ppp},                        // P hangs up and selects ppp
		{"hotkeys = yes", ppp + "[item bbs]\nkey = BBS\ncommand = true\n"},
	} {
		if _, err := load(c.menu, c.items); err == nil {
			t.Errorf("%q %q: no error", c.menu, c.items)
		}
	}
	if _, err := load("hotkeys = yes", ppp); err != nil {
		t.Errorf("hotkeys with one-character keys: %v", err)
	}
}

// runGo: the go command with args, its output
func runGo(args ...string) ([]byte, error) {
	return exec.Command("go", args...).CombinedOutput()
}
