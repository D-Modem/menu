// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestINI: the configuration file's sections, keys in lower case and
// quoted values
func TestINI(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.conf")
	os.WriteFile(p, []byte("; comment\n[general]\nauth = file, sql\nbanner = \"  hi  \"\n[item  bbs]\nKey = b\n"), 0o600)
	c, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if g := c.Section("general"); g.Get("banner", "") != "  hi  " || len(Fields(g.Get("auth", ""))) != 2 {
		t.Errorf("general: %v", g.Vals)
	}
	if c.Section("item bbs").Get("key", "") != "b" {
		t.Errorf("item: %v", c.Section("item bbs").Vals)
	}
}

// TestStripComment: inline comments end a value after whitespace, not
// inside quotes or a word; \; and \# are kept as text
func TestStripComment(t *testing.T) {
	for in, want := range map[string]string{
		"P ; PPP":                  "P ",
		"P # PPP":                  "P ",
		"a;b":                      "a;b",
		"echo a; echo b":           "echo a; echo b",
		`"a ; b" ; c`:              `"a ; b" `,
		`x \; y`:                   "x ; y",
		`x \# y`:                   "x # y",
		"; all comment":            "",
		"(&(uid={user})(#x))":      "(&(uid={user})(#x))",
		"exec pppd ipparam {user}": "exec pppd ipparam {user}",
	} {
		if got := StripComment(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// TestDurations: time_warning's list of durations, longest first, and
// none for 0 or none
func TestDurations(t *testing.T) {
	for in, want := range map[string]string{
		"5m 1m":    "[5m0s 1m0s]",
		"60, 300":  "[5m0s 1m0s]",
		"1m 5m 1m": "[5m0s 1m0s]",
		"90s":      "[1m30s]",
		"0":        "[]",
		"none":     "[]",
		"":         "[]",
	} {
		s := &Section{Name: "general", Vals: map[string]string{"time_warning": in}}
		got, err := s.Durations("time_warning", "2m")
		if err != nil || fmt.Sprint(got) != want {
			t.Errorf("%q: %v %v, want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"5m soon", "-1m", "1m 0"} {
		s := &Section{Name: "general", Vals: map[string]string{"time_warning": bad}}
		if _, err := s.Durations("time_warning", "2m"); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
	if got, _ := (&Section{Name: "general", Vals: map[string]string{}}).Durations("time_warning", "2m"); fmt.Sprint(got) != "[2m0s]" {
		t.Errorf("the default: %v", got)
	}
}
