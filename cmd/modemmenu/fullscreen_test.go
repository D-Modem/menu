// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/D-Modem/menu/internal/tui"
)

// TestFullScreenMenu: an ANSI caller at 2400 bit/s gets the menu full
// screen (in code page 437, the desk plain): an item by its key, the
// pause after it, the sysop's setup and back, goodbye by its key; a
// caller whose terminal says nothing gets the menu of lines
func TestFullScreenMenu(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\nbob:{PLAIN}pw\n"), 0o600)
	conf := filepath.Join(dir, "modemmenu.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
fail_delay = 0
motd = "The line is down on Sunday."
[file]
path = %s
[groups]
sysops = alice
[item board]
key = B
label = The board
help = Messages and files
mode = exec
command = echo hello from the board
after = menu
[item setup]
key = Z
label = Setup
mode = setup
groups = sysops
`, users)), 0o600)

	start := func(answer bool) *caller {
		m, s := testPty(t)
		t.Cleanup(func() { m.Close() })
		cmd := exec.Command(bin, "-config", conf)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
		cmd.Env = append(os.Environ(), "DMODEM_RATE=2400")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		s.Close()
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		if answer {
			return newCaller(t, m)
		}
		return newTermCaller(t, m, func([]byte) []byte { return nil })
	}

	c := start(true)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	// the desk, cleared in its blue, the motd in a box over the menu
	got := c.expect("The line is down on Sunday.")
	if !strings.Contains(got, "\x1b[0m\x1b[0;37;44m\x1b[2J") {
		t.Errorf("no desk: %q", got)
	}
	got += c.expect(" OK ")
	if strings.Contains(got, "░") || strings.Contains(got, "\xb0") || strings.Contains(got, "╔") {
		t.Errorf("a shaded desk, or UTF-8, at 2400 bit/s: %q", got)
	}
	if !strings.Contains(got, "\xc9\xcd") { // ╔═ in code page 437
		t.Errorf("no code page 437 frame: %q", got)
	}
	c.send("\r")
	c.expect("The board")
	c.expect("Messages and files")
	c.expect("Goodbye (hang up)")
	c.send("b")
	c.expect("\x1b[2J")
	c.expect("hello from the board")
	c.expect("Press any key for the menu")
	c.send(" ")
	c.expect("The board")

	// the setup, and F9 back to the menu
	c.send("z")
	c.expect("modemmenu Setup")
	c.expect("Login and messages")
	c.send("\x1b[20~")
	c.expect("The board")
	c.send("g")
	c.expect("Goodbye.")

	// bob is no sysop: no setup on his menu, and its key does nothing
	c = start(true)
	c.expect("login: ")
	c.send("bob\r")
	c.expect("Password: ")
	c.send("pw\r")
	c.expect(" OK ")
	c.send("\r")
	got = c.expect("Goodbye (hang up)")
	if strings.Contains(got, "Setup") {
		t.Errorf("bob sees the setup: %q", got)
	}
	c.send("z")
	time.Sleep(300 * time.Millisecond)
	c.send("g")
	got = c.expect("Goodbye.")
	if strings.Contains(got, "modemmenu Setup") {
		t.Errorf("bob got the setup: %q", got)
	}

	// a terminal that answers nothing: the menu of lines
	c = start(false)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("[B] The board")
	c.expect("Selection: ")
	c.send("g\r")
	c.expect("Goodbye.")
}

// TestSetupNeedsSysops: an item of mode = setup is for some only
func TestSetupNeedsSysops(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		item, err string
	}{
		{"mode = setup", "needs users or groups"},
		{"mode = setup\nguest = yes\nusers = alice", "not for guests"},
		{"mode = setup\nusers = alice", ""},
	} {
		p := filepath.Join(dir, "c.conf")
		os.WriteFile(p, []byte("[general]\nlog = none\nauth = file\n[file]\npath = /dev/null\n[item s]\nkey = S\n"+c.item+"\n"), 0o600)
		_, err := loadSettings(p)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%q: %v", c.item, err)
		}
	}
}

// keyLine: keys queued for the setup, what it writes kept
type keyLine struct {
	in  []byte
	out bytes.Buffer
}

func (f *keyLine) ReadOne(time.Duration) (byte, error) {
	if len(f.in) == 0 {
		return 0, tui.ErrTimeout
	}
	b := f.in[0]
	f.in = f.in[1:]
	return b, nil
}

func (f *keyLine) WriteRaw(s string) error { f.out.WriteString(s); return nil }

// TestSetup: values changed through the screens and saved, the file's
// comments and order kept; a section added, an item renamed in [menu]
// items too; a file that does not check is not saved without asking
func TestSetup(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	conf := filepath.Join(dir, "modemmenu.conf")
	orig := fmt.Sprintf(`; my line
[general]
log = none
auth = file
; how many tries
attempts = 3 ; three
[file]
path = %s
[menu]
items = bbs
[item bbs]
key = B
label = Board
mode = exec
command = true
`, users)
	os.WriteFile(conf, []byte(orig), 0o640)

	// an Esc and what follows must not be taken as Alt: they come apart
	const esc = "\x1b\x00"
	const (
		up, enter, f9, f10 = "\x1b[A", "\r", "\x1b[20~", "\x1b[21~"
	)
	keys := strings.Join([]string{
		enter,           // Login and messages
		"a", "a", enter, // auth, attempts: typed
		"\x15", "5", enter, //
		esc,        // back to the first window
		"g", enter, // Guest account
		enter, up, enter, // enabled: from the list, no
		esc,                    //
		"m", "m", enter, enter, // Menu items, bbs
		"\x1b[F", up, enter, "\x15board", enter, // End, Rename
		esc, esc, //
		f10, f9,
	}, "")
	f := &keyLine{in: []byte(keys)}
	d := &tui.Desk{Line: f, Rows: 25, Cols: 80, Charset: tui.UTF8}
	u, err := newSetup(d, conf)
	if err != nil {
		t.Fatal(err)
	}
	u.size = func() (int, int) { return 25, 80 }
	if err := u.run(); err != nil {
		if scr := d.Screen(); scr != nil {
			t.Log("\n" + scr.Text())
		}
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	want := fmt.Sprintf(`; my line
[general]
log = none
auth = file
; how many tries
attempts = 5 ; three
[file]
path = %s
[menu]
items = board
[item board]
key = B
label = Board
mode = exec
command = true

[guest]
enabled = no
`, users)
	if string(b) != want {
		t.Errorf("saved:\n%s\nwant:\n%s", b, want)
	}
	if st, _ := os.Stat(conf); st.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", st.Mode())
	}
	if _, err := loadSettings(conf); err != nil {
		t.Errorf("does not load: %v", err)
	}

	// a mistake: the check says, Back keeps the file as it is
	keys = strings.Join([]string{enter, "a", "a", enter, "\x15", "many", enter, f10, enter, f9, "d"}, "")
	f.in = []byte(keys)
	f.out.Reset()
	u, _ = newSetup(d, conf)
	if err := u.run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), "not a number") {
		t.Errorf("the check's error not shown")
	}
	if b2, _ := os.ReadFile(conf); string(b2) != want {
		t.Errorf("saved after a failed check:\n%s", b2)
	}
}
