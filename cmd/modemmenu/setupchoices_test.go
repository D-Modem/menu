// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/D-Modem/menu/internal/confdoc"
	"github.com/D-Modem/menu/internal/tui"
	"github.com/D-Modem/menu/samples"
)

// TestEverySettingDescribed: each section and key of the sample, which
// is what the setup shows, has a description of what it does; and no
// description is left for a key the sample does not have
func TestEverySettingDescribed(t *testing.T) {
	have := map[string]bool{}
	for _, kd := range confdoc.Parse(samples.Conf) {
		have[kd.Name] = true
		if setupDesc(kd.Name, "") == "" {
			t.Errorf("[%s]: no description", kd.Name)
		}
		for _, k := range kd.Keys {
			have[kd.Name+"."+k.Name] = true
			if setupDesc(kd.Name, k.Name) == "" {
				t.Errorf("[%s] %s: no description", kd.Name, k.Name)
			}
		}
	}
	for k := range setupDescs {
		if !have[k] {
			t.Errorf("%s: described, but not in the sample", k)
		}
	}
}

// setupFor: a setup of a small file, with keys to type at it
func setupFor(t *testing.T, conf string) (*setupUI, *keyLine) {
	t.Helper()
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\nbob:{PLAIN}x\n"), 0o600)
	path := filepath.Join(dir, "modemmenu.conf")
	os.WriteFile(path, []byte(fmt.Sprintf(conf, users)), 0o640)
	f := &keyLine{}
	d := &tui.Desk{Line: f, Rows: 25, Cols: 80, Charset: tui.UTF8}
	u, err := newSetup(d, path)
	if err != nil {
		t.Fatal(err)
	}
	return u, f
}

const choicesConf = `[general]
log = none
auth = file
[file]
path = %s
[groups]
sysops = alice
[menu]
items = bbs passwd
[item bbs]
key = B
label = Board
command = true
[item passwd]
key = PW
label = Password
mode = passwd
`

const (
	kDown, kEnter, kEsc = "\x1b[B", "\r", "\x1b\x00"
)

// TestSetupChoices: the lists a key is picked from, with their notes
func TestSetupChoices(t *testing.T) {
	u, _ := setupFor(t, choicesConf)
	kd := func(n string) *confdoc.Kind { return u.byKind[n] }
	get := func(kind, sec, key string) *choiceSet {
		return u.choicesFor(sec, kd(kind), kd(kind).ByKey[key])
	}

	// closed, with notes: nothing else is typed
	m := get("item", "item bbs", "mode")
	if m.open || m.multi || !m.has("passwd") || !m.has("setup") {
		t.Errorf("mode: %+v", m)
	}
	// the items, for [menu] items: checked off, with their labels
	it := get("menu", "menu", "items")
	if !it.multi || len(it.list) != 2 || it.list[0] != (choice{"bbs", "Board"}) || it.list[1].note != "Password (passwd)" {
		t.Errorf("items: %+v", it)
	}
	// the users file's users; [groups]'s groups
	if us := get("item", "item bbs", "users"); !us.has("alice") || !us.has("bob") || !us.multi {
		t.Errorf("users: %+v", us)
	}
	if gs := get("item", "item bbs", "groups"); !gs.has("sysops") || gs.list[0].note != "[groups]: alice" {
		t.Errorf("groups: %+v", gs)
	}
	// keys: what is taken says by what
	keys := get("item", "item passwd", "key")
	notes := map[string]string{}
	for _, c := range keys.list {
		notes[c.value] = c.note
	}
	if notes["P"] != "free" || notes["S"] != "free" || notes["B"] != "" {
		t.Errorf("passwd's keys: %v", notes)
	}
	keys = get("item", "item bbs", "key")
	for _, c := range keys.list {
		notes[c.value] = c.note
	}
	if notes["B"] != "free" || notes["O"] != "free" {
		t.Errorf("bbs's keys: %v", notes)
	}
	u.doc.Set("item bbs", "label", "Go play")
	for _, c := range get("item", "item bbs", "key").list {
		if c.value == "G" && c.note != "the hangup" {
			t.Errorf("G: %q", c.note)
		}
	}
	// a yes-or-no
	if b := get("guest", "guest", "enabled"); b.open || len(b.list) != 2 {
		t.Errorf("enabled: %+v", b)
	}
	// F1 lists them
	h := keyFullHelp(setupDesc("item", "mode"), kd("item").ByKey["mode"], m)
	if !strings.Contains(h, "One of:") || !strings.Contains(h, "passwd: built in: change your password") || !strings.HasPrefix(h, "What the item does") {
		t.Errorf("help:\n%s", h)
	}
}

// TestSetupMulti: [menu] items checked off: one unchecked, the order kept
func TestSetupMulti(t *testing.T) {
	u, f := setupFor(t, choicesConf)
	u.open("menu")
	p := u.stack[len(u.stack)-1]
	u.fill(p)
	f.in = []byte(" " + kEnter) // bbs off
	if err := u.edit(p, "items", false); err != nil {
		t.Fatal(err)
	}
	if v, _ := u.doc.Get("menu", "items"); v != "passwd" {
		t.Errorf("items = %q", v)
	}
	f.in = []byte(kDown + " " + kEnter) // bbs back on, after passwd
	u.edit(p, "items", false)
	if v, _ := u.doc.Get("menu", "items"); v != "passwd bbs" {
		t.Errorf("items = %q", v)
	}
}

// TestSetupAddItem: a menu item from the sample's: its keys, a free key
// when the sample's is taken, in the menu's items
func TestSetupAddItem(t *testing.T) {
	u, f := setupFor(t, choicesConf)
	u.open("item")
	p := u.stack[len(u.stack)-1]
	u.fill(p)
	add := func(label string) {
		t.Helper()
		p.l.Cur = len(p.l.Items) - 1
		if p.l.Items[p.l.Cur].ID != "add" {
			t.Fatalf("not the add line: %+v", p.l.Items[p.l.Cur])
		}
		at := -1
		for i, tp := range itemTemplates(samples.Conf) {
			if tp.get("label") == label {
				at = i + 1
			}
		}
		if at < 0 {
			t.Fatalf("no %q in the sample", label)
		}
		f.in = []byte(strings.Repeat(kDown, at) + kEnter + kEnter) // the template; its name as offered
		if err := u.pick(p, false); err != nil {
			t.Fatal(err)
		}
		u.stack = u.stack[:len(u.stack)-1]
		u.fill(p)
	}
	add("Last callers")
	if v, _ := u.doc.Get("item callers", "mode"); v != "callers" {
		t.Errorf("callers' mode %q", v)
	}
	if v, _ := u.doc.Get("item callers", "key"); v != "LC" {
		t.Errorf("callers' key %q", v)
	}
	add("Bulletin board") // B is bbs's
	if v, _ := u.doc.Get("item bbs2", "key"); v != "S" {
		t.Errorf("bbs2's key %q", v)
	}
	if v, _ := u.doc.Get("item bbs2", "mode"); v != "rlogin" {
		t.Errorf("bbs2's mode %q", v)
	}
	if _, ok := u.doc.Get("item bbs2", "send"); ok {
		t.Errorf("the sample's other way (send) taken as its own")
	}
	if v, _ := u.doc.Get("menu", "items"); v != "bbs passwd callers bbs2" {
		t.Errorf("items = %q", v)
	}
	if err := checkConf(u.path, u.doc.String()); err != nil {
		t.Errorf("does not check: %v", err)
	}
}

// TestItemTemplates: the sample's items, each with its own keys
func TestItemTemplates(t *testing.T) {
	ts := itemTemplates(samples.Conf)
	by := map[string]itemTemplate{}
	for _, x := range ts {
		by[x.get("label")] = x
	}
	if p := by["PPP (dial-up networking)"]; p.get("time_warning") != "no" || !strings.HasPrefix(p.get("command"), "exec /usr/sbin/pppd") {
		t.Errorf("ppp: %+v", p)
	}
	if s := by["Setup"]; s.get("mode") != "setup" || s.get("groups") != "sysops" {
		t.Errorf("setup: %+v", s)
	}
	if len(ts) < 10 {
		t.Errorf("%d templates", len(ts))
	}
}
