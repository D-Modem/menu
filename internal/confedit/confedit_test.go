// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package confedit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/D-Modem/menu/internal/config"
)

const sample = `; the file's comment

[general]
; the log
log = syslog ; or a file
banner = "\r\nHi\r\n"
attempts=3

; the menu
[menu]
title = Main

; a BBS
[item bbs]
key = B
label = Board

; the shell
[item shell]
key = S
`

// TestSet: values changed in place with their comments and quotes, new
// keys after their section's last, new sections at the end
func TestSet(t *testing.T) {
	d := Parse(sample)
	d.Set("general", "log", "/var/log/modemmenu.log")
	d.Set("general", "banner", "Hello")
	d.Set("general", "attempts", "5")
	d.Set("general", "motd", " spaced ")
	d.Set("menu", "hotkeys", "yes")
	d.Set("aol", "enabled", "yes")
	d.Set("item bbs", "help", "a; b")
	want := `; the file's comment

[general]
; the log
log = /var/log/modemmenu.log ; or a file
banner = "Hello"
attempts=5
motd = " spaced "

; the menu
[menu]
title = Main
hotkeys = yes

; a BBS
[item bbs]
key = B
label = Board
help = a; b

; the shell
[item shell]
key = S

[aol]
enabled = yes
`
	if got := d.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// and the configuration reads what was set
	p := filepath.Join(t.TempDir(), "c.conf")
	os.WriteFile(p, []byte(d.String()), 0o600)
	c, err := config.Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][3]string{{"general", "log", "/var/log/modemmenu.log"}, {"general", "motd", " spaced "},
		{"general", "banner", "Hello"}, {"item bbs", "help", "a; b"}, {"aol", "enabled", "yes"}} {
		if v := c.Section(kv[0]).Get(kv[1], "?"); v != kv[2] {
			t.Errorf("[%s] %s = %q, want %q", kv[0], kv[1], v, kv[2])
		}
		if v, _ := d.Get(kv[0], kv[1]); v != kv[2] {
			t.Errorf("Get [%s] %s = %q, want %q", kv[0], kv[1], v, kv[2])
		}
	}
	for _, v := range []string{"a ; b", "x # y", `"q"`, ""} {
		d.Set("general", "motd", v)
		os.WriteFile(p, []byte(d.String()), 0o600)
		c, _ := config.Parse(p)
		if got := c.Section("general").Get("motd", "?"); got != v {
			t.Errorf("motd %q read back as %q", v, got)
		}
	}
}

// TestRemove: a key and a section out, the next one's comment kept
func TestRemove(t *testing.T) {
	d := Parse(sample)
	d.Unset("general", "attempts")
	d.RemoveSection("item bbs")
	d.RenameSection("item shell", "item sh")
	want := `; the file's comment

[general]
; the log
log = syslog ; or a file
banner = "\r\nHi\r\n"

; the menu
[menu]
title = Main

; the shell
[item sh]
key = S
`
	if got := d.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	d.RemoveSection("item sh")
	if got := d.String(); got != "; the file's comment\n\n[general]\n; the log\nlog = syslog ; or a file\nbanner = \"\\r\\nHi\\r\\n\"\n\n; the menu\n[menu]\ntitle = Main\n" {
		t.Errorf("got\n%s", got)
	}
	if s := d.Sections(); len(s) != 2 || s[1] != "menu" {
		t.Errorf("sections %q", s)
	}
}
