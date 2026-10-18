// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestParseVT(t *testing.T) {
	for _, c := range []struct {
		what, reply, name, term string
		level                   int
	}{
		{"xterm", "\x1b[?64;1;2;6;9;15;18;21;22c\x1b[>41;390;0c\x1bP>|XTerm(390)\x1b\\\x1b[50;132R", "XTerm(390)", "xterm", 4},
		{"xterm without XTVERSION", "\x1b[?65;1;9c\x1b[>41;390;0c\x1b[24;80R", "xterm (patch 390)", "xterm", 5},
		{"a VT220", "\x1b[?62;1;2;6;8;9;15c\x1b[>1;10;0c\x1b[24;80R", "VT220", "vt220", 2},
		{"a VT520", "\x1b[?65;1;2;6;7;8;9;12;18;19;21;22;23;24;42;44;45;46c\x1b[>64;20;0c\x1b[24;80R", "VT520", "vt520", 5},
		{"a VT100", "\x1b[?1;2c\x1b[24;80R", "VT100", "vt100", 1},
		{"a VT102", "\x1b[?6c\x1b[24;80R", "VT102", "vt102", 1},
		{"SyncTERM", "\x1b[=67;84;101;114;109;1;316c\x1b[24;80R", "SyncTERM (CTerm 1.316)", "syncterm", 0},
		{"VTE", "\x1b[?65;1;9c\x1b[>65;6800;1c\x1b[24;80R", "VTE", "xterm", 5},
		{"an answerback", "MYTERM\x1b[?62;1c\x1b[24;80R", "MYTERM", "vt220", 2},
		{"nothing said", "\x1b[24;80R", "", "ansi", 0},
	} {
		in := parseVT([]byte(c.reply))
		if in.name() != c.name || in.term() != c.term || in.level() != c.level {
			t.Errorf("%s: %q %q level %d, want %q %q %d", c.what, in.name(), in.term(), in.level(), c.name, c.term, c.level)
		}
	}
}

func TestStripTelnet(t *testing.T) {
	in := []byte("a\xff\xfb\x18b\xff\xfa\x18\x00ANSI\xff\xf0c\xff\xffd")
	if got := string(stripTelnet(in)); got != "abc\xffd" {
		t.Errorf("%q", got)
	}
}

func TestPETSCIIText(t *testing.T) {
	if got := string(petsciiOut("Hi you\r\n\x1b[1;33mX\b \b\n")); got != "\xc8I YOU\r\xd8\x9d \x9d\r" {
		t.Errorf("out %q", got)
	}
	var in []byte
	for _, c := range []byte("HI\xc8\x14\r") {
		in = append(in, petsciiIn(c))
	}
	if string(in) != "hiH\x7f\r" {
		t.Errorf("in %q", in)
	}
}

// the terminal's details from an upstream menu's header, not asked again
func TestTerminalHeader(t *testing.T) {
	env := strings.Join(headerEnv(map[string]string{"MODEM_TERMINAL": "vt", "MODEM_TERM": "vt220",
		"MODEM_TERM_NAME": "VT220", "MODEM_ROWS": "24", "MODEM_COLS": "80"}, false), "\n") + "\n"
	for _, want := range []string{"MODEMMENU_UPSTREAM_TERMINAL=vt\n", "MODEMMENU_UPSTREAM_TERM=vt220\n",
		"MODEMMENU_UPSTREAM_TERM_NAME=VT220\n", "MODEMMENU_UPSTREAM_ROWS=24\n", "MODEMMENU_UPSTREAM_COLS=80\n"} {
		if !strings.Contains(env, want) {
			t.Errorf("no %s in\n%s", want, env)
		}
	}
}

// terminalConf: alice/secret, and an item that shows what it was told of
// the terminal
func terminalConf(t *testing.T, dir, extra string) (string, string) {
	bin := filepath.Join(dir, "modemmenu")
	if _, err := os.Stat(bin); err != nil {
		if out, err := runGo("build", "-o", bin, "."); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	p := filepath.Join(dir, fmt.Sprintf("t%d.conf", len(extra)))
	os.WriteFile(p, []byte(fmt.Sprintf(`[general]
log = none
fail_delay = 0
[file]
path = %s
[terminal]
timeout = 300ms
%s
[item e]
command = env | grep -E '^(TERM|MODEM_TERM|MODEM_TERMINAL|MODEM_TERM_NAME|MODEM_ROWS|MODEM_COLS)=' | sort; echo size $(stty size); echo END
after = hangup
`, users, extra)), 0o600)
	return bin, p
}

// replies: a terminal that answers each question it is asked, by what it
// is asked
func replies(qa ...string) func([]byte) []byte {
	return func(q []byte) []byte {
		var out []byte
		for i := 0; i+1 < len(qa); i += 2 {
			if bytes.Contains(q, []byte(qa[i])) {
				out = append(out, qa[i+1]...)
			}
		}
		return out
	}
}

func TestDetectTerminal(t *testing.T) {
	dir := t.TempDir()
	bin, conf := terminalConf(t, dir, "")
	for _, c := range []struct {
		what   string
		answer func([]byte) []byte
		want   []string
	}{
		{"xterm", replies("\x1b[c", "\x1b[?64;1;2;6;9;15;18;21;22c", "\x1b[>c", "\x1b[>41;390;0c",
			"\x1b[>0q", "\x1bP>|XTerm(390)\x1b\\", "\x1b[6n", "\x1b[50;132R"),
			[]string{"MODEM_TERMINAL=vt", "MODEM_TERM=xterm", "MODEM_TERM_NAME=XTerm(390)",
				"MODEM_ROWS=50", "MODEM_COLS=132", "TERM=xterm", "size 50 132"}},
		{"a VT220", replies("\x05", "VT220-1", "\x1b[c", "\x1b[?62;1;2;6;8;9;15c", "\x1b[>c", "\x1b[>1;10;0c", "\x1b[6n", "\x1b[24;80R"),
			[]string{"MODEM_TERMINAL=vt", "MODEM_TERM=vt220", "MODEM_TERM_NAME=VT220", "TERM=vt220", "size 24 80"}},
		{"SyncTERM", replies("\x1b[c", "\x1b[=67;84;101;114;109;1;316c", "\x1b[6n", "\x1b[25;80R"),
			[]string{"MODEM_TERMINAL=vt", "MODEM_TERM=syncterm", "MODEM_TERM_NAME=SyncTERM (CTerm 1.316)", "size 25 80"}},
		{"ANSI that says nothing more", replies("\x1b[6n", "\x1b[24;80R"),
			[]string{"MODEM_TERMINAL=ansi", "MODEM_TERM=ansi", "TERM=ansi", "size 24 80"}},
		{"a VT52", replies("\x1bZ", "\x1b/K"),
			[]string{"MODEM_TERMINAL=vt52", "MODEM_TERM=vt52", "MODEM_TERM_NAME=VT52", "size 24 80"}},
		{"a plain terminal", replies(),
			[]string{"MODEM_TERMINAL=ascii", "MODEM_TERM=dumb", "TERM=dumb"}},
	} {
		m, s := testPty(t)
		cmd := startOn(t, bin, conf, s)
		cl := newTermCaller(t, m, c.answer)
		cl.expect("login: ")
		cl.send("alice\r")
		cl.expect("Password: ")
		cl.send("secret\r")
		cl.expect("Selection: ")
		cl.send("e\r")
		got := cl.expect("END")
		for _, w := range c.want {
			if !strings.Contains(got, w+"\n") && !strings.Contains(got, w+"\r\n") {
				t.Errorf("%s: no %s in %q", c.what, w, got)
			}
		}
		if c.what == "a plain terminal" && !strings.Contains(got, "MODEM_TERM_NAME=\n") {
			t.Errorf("%s: a name in %q", c.what, got)
		}
		waitExit(t, cmd)
		m.Close()
	}
}

// what the caller typed while asked is the login's
func TestDetectTypeAhead(t *testing.T) {
	dir := t.TempDir()
	bin, conf := terminalConf(t, dir, "")
	m, s := testPty(t)
	cmd := startOn(t, bin, conf, s)
	// a plain terminal, and a caller who types at once
	m.Write([]byte("alice\r"))
	cl := newTermCaller(t, m, replies())
	cl.expect("Password: ")
	cl.send("secret\r")
	cl.expect("Selection: ")
	cl.send("g\r")
	waitExit(t, cmd)
}

func TestDetectPETSCII(t *testing.T) {
	dir := t.TempDir()
	bin, conf := terminalConf(t, dir, "")

	// a shifted letter: only a Commodore sends it
	m, s := testPty(t)
	cmd := startOn(t, bin, conf, s)
	cl := newTermCaller(t, m, replies())
	cl.expect("login: ")
	cl.send("\xc1")
	cl.expect("\x0e")
	cl.expect("\rLOGIN: ")
	// its unshifted keys: lower case
	cl.send("ALICE\r")
	cl.expect("ALICE\r")
	cl.expect("\xd0ASSWORD: ")
	cl.send("SECRET\r")
	cl.expect("\xd7ELCOME, ALICE.")
	cl.send("E\r")
	got := cl.expect("END")
	for _, w := range []string{"MODEM_TERMINAL=petscii", "MODEM_ROWS=25", "MODEM_COLS=40", "size 25 40"} {
		if !strings.Contains(got, w) {
			t.Errorf("no %s in %q", w, got)
		}
	}
	waitExit(t, cmd)
	m.Close()

	// a name all in capitals: DEL tells
	m, s = testPty(t)
	cmd = startOn(t, bin, conf, s)
	cl = newTermCaller(t, m, replies())
	cl.expect("login: ")
	cl.send("ALICE\r")
	cl.expect("PRESS DEL (OR BACKSPACE): ")
	cl.send("\x14")
	cl.expect("\x0e")
	cl.expect("LOGIN: ALICE\r")
	cl.expect("\xd0ASSWORD: ")
	cl.send("SECRET\r")
	cl.expect("\xd7ELCOME, ALICE.")
	cmd.Process.Kill()
	m.Close()

	// or that it is no Commodore: the name as typed
	m, s = testPty(t)
	cmd = startOn(t, bin, conf, s)
	cl = newTermCaller(t, m, replies())
	cl.expect("login: ")
	cl.send("ALICE\r")
	cl.expect("PRESS DEL (OR BACKSPACE): ")
	cl.send("\r")
	cl.expect("Password: ")
	cmd.Process.Kill()
	m.Close()

	// an ANSI terminal is asked nothing of it
	m, s = testPty(t)
	cmd = startOn(t, bin, conf, s)
	cl = newCaller(t, m)
	cl.expect("login: ")
	cl.send("ALICE\r")
	cl.expect("Password: ")
	cmd.Process.Kill()
	m.Close()

	// petscii = yes: every caller, asked nothing
	bin, conf = terminalConf(t, dir, "petscii = yes")
	m, s = testPty(t)
	cmd = startOn(t, bin, conf, s)
	cl = newTermCaller(t, m, replies())
	if got := cl.expect("LOGIN: "); strings.Contains(got, "\x1b") || !strings.HasPrefix(got, "\x0e") {
		t.Errorf("%q", got)
	}
	cmd.Process.Kill()
	m.Close()

	// an upstream menu said
	bin, conf = terminalConf(t, dir, "")
	m, s = testPty(t)
	cmd = startOn(t, bin, conf, s, "MODEMMENU_UPSTREAM_TERMINAL=petscii", "MODEMMENU_UPSTREAM_TERM=dumb")
	cl = newTermCaller(t, m, replies())
	if got := cl.expect("LOGIN: "); strings.Contains(got, "\x1b") {
		t.Errorf("asked: %q", got)
	}
	cmd.Process.Kill()
	m.Close()
}

func TestTN3270(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Write([]byte("3270 HERE"))
			b := make([]byte, 4)
			for got := 0; got < 4; {
				n, err := conn.Read(b[got:])
				if err != nil {
					break
				}
				got += n
			}
			if string(b) == "ping" {
				conn.Write([]byte("pong"))
			}
			conn.Close()
		}
	}()
	dir := t.TempDir()
	bin, conf := terminalConf(t, dir, "tn3270 = tcp:"+ln.Addr().String())

	// a 3270: to the server
	m, s := testPty(t)
	cmd := startOn(t, bin, conf, s)
	cl := newTermCaller(t, m, replies("\xff\xfd\x18", "\xff\xfb\x18",
		"\xff\xfa\x18\x01\xff\xf0", "\xff\xfa\x18\x00IBM-3278-2-E\xff\xf0", "\xff\xfe\x18", "\xff\xfc\x18"))
	if got := cl.expect("3270 HERE"); strings.Contains(got, "login") {
		t.Errorf("%q", got)
	}
	cl.send("ping")
	cl.expect("pong")
	waitExit(t, cmd)
	m.Close()

	// a telnet client of another terminal type, and a terminal that is
	// no telnet client: the login
	for _, answer := range []func([]byte) []byte{
		replies("\xff\xfd\x18", "\xff\xfb\x18", "\xff\xfa\x18\x01\xff\xf0", "\xff\xfa\x18\x00ANSI\xff\xf0", "\x1b[6n", "\x1b[24;80R"),
		replies(),
	} {
		m, s := testPty(t)
		cmd := startOn(t, bin, conf, s)
		cl := newTermCaller(t, m, answer)
		cl.expect("login: ")
		cl.send("alice\r")
		cl.expect("Password: ")
		cmd.Process.Kill()
		m.Close()
	}
}

// startOn: modemmenu on the slave of a pty
func startOn(t *testing.T, bin, conf string, s *os.File, env ...string) *exec.Cmd {
	cmd := exec.Command(bin, "-config", conf)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

// a dialer that starts PPP at once: its frames while the terminal is
// asked end the questions, and are the AutoPPP's
func TestDetectPPP(t *testing.T) {
	dir := t.TempDir()
	bin, conf := terminalConf(t, dir, "[autoppp]\nenabled = yes\ncommand = echo PPP STARTED")
	m, s := testPty(t)
	cmd := startOn(t, bin, conf, s)
	m.Write([]byte("\x7e\xff\x7d\x23\xc0\x21\x7d\x21\x7d\x21\x7d\x20\x7d\x34\x7e"))
	cl := newTermCaller(t, m, replies())
	got := cl.expect("PPP STARTED")
	if strings.Contains(got, "\x1bZ") || strings.Contains(got, "\x05") {
		t.Errorf("asked on: %q", got)
	}
	waitExit(t, cmd)
}
