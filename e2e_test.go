// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testPty: master and slave of a new pty, raw as app_dmodem makes it
func testPty(t *testing.T) (*os.File, *os.File) {
	m, s, err := openPty()
	if err != nil {
		t.Skip("no pty:", err)
	}
	return m, s
}

// caller: the remote end of the line
type caller struct {
	t   *testing.T
	m   io.ReadWriter
	in  chan []byte
	buf bytes.Buffer
}

// newCaller: a caller with an ANSI terminal of 24 lines of 80, which
// answers where its cursor is (and nothing else it is asked)
func newCaller(t *testing.T, m io.ReadWriter) *caller {
	return newTermCaller(t, m, func(q []byte) []byte {
		if bytes.Contains(q, []byte("\x1b[6n")) {
			return []byte("\x1b[24;80R")
		}
		return nil
	})
}

// newTermCaller: a caller whose terminal answers what it is asked: answer
// gets what came (with the end of what came before), and says what goes
// back
func newTermCaller(t *testing.T, m io.ReadWriter, answer func([]byte) []byte) *caller {
	c := &caller{t: t, m: m, in: make(chan []byte, 64)}
	go func() {
		var tail []byte
		for {
			b := make([]byte, 1024)
			n, err := m.Read(b)
			if n > 0 {
				q := append(tail, b[:n]...)
				if a := answer(q); a != nil {
					m.Write(a)
					tail = nil
				} else {
					tail = append([]byte{}, q[max(0, len(q)-8):]...)
				}
				c.in <- b[:n]
			}
			if err != nil {
				close(c.in)
				return
			}
		}
	}()
	return c
}

// expect: what the line says up to and including s; the test fails
// after 10 seconds without it
func (c *caller) expect(s string) string {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for !strings.Contains(c.buf.String(), s) {
		select {
		case b, ok := <-c.in:
			if !ok {
				c.t.Fatalf("waiting for %q: hung up; got %q", s, c.buf.String())
			}
			c.buf.Write(b)
		case <-timeout:
			c.t.Fatalf("waiting for %q: timeout; got %q", s, c.buf.String())
		}
	}
	i := strings.Index(c.buf.String(), s) + len(s)
	got := c.buf.String()[:i]
	rest := c.buf.String()[i:]
	c.buf.Reset()
	c.buf.WriteString(rest)
	return got
}

// send: s typed on the line
func (c *caller) send(s string) {
	c.m.Write([]byte(s))
}

// TestLine: a call on a pty: login, the menu, and an rlogin BBS that
// gets the caller's credentials
func TestLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// an rlogin BBS: checks the handshake, says who logged in, ends on Q
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hello := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b := make([]byte, 256)
		got := ""
		for strings.Count(got, "\x00") < 4 {
			n, err := conn.Read(b)
			if err != nil {
				return
			}
			got += string(b[:n])
		}
		hello <- got
		parts := strings.Split(got, "\x00")
		conn.Write([]byte("\x00Welcome to the BBS, " + parts[2] + "\r\n"))
		for {
			n, err := conn.Read(b)
			if err != nil || bytes.ContainsRune(b[:n], 'Q') {
				conn.Write([]byte("Bye from the BBS\r\n"))
				return
			}
		}
	}()

	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "motd"), []byte("Line closed Sunday\n"), 0o600)
	envOut := filepath.Join(dir, "env")
	conf := filepath.Join(dir, "modemmenu.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
banner = "Test line\r\n"
motd = "MOTD for {user}\r\n"
motd_file = %s
[file]
path = %s
[item bbs]
key = B
label = BBS
mode = rlogin
host = 127.0.0.1
port = %d
after = menu
[item shell]
key = SH
label = Shell
mode = exec
tty = sane
after = menu
command = PS1='sh> ' exec sh -i
[item info]
key = I
aliases = details
label = Info
mode = exec
session_file = yes
tty = sane
command = env > %s; cat "$MODEM_SESSION_FILE" >> %s; echo args: {user} {baud} {dialed}; read x; echo "read $x"
`, filepath.Join(dir, "log"), filepath.Join(dir, "motd"), users, ln.Addr().(*net.TCPAddr).Port, envOut, envOut)), 0o600)

	m, s := testPty(t)
	defer m.Close()
	cmd := exec.Command(bin, "-config", conf)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.Env = append(os.Environ(), "DMODEM_RATE=33600/31200", "DMODEM_CALLERID=5559999",
		"DMODEM_DIALED=5551000", "DMODEM_CHANNEL=Local/s@test", "TMPDIR="+dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	defer cmd.Process.Kill()

	c := newCaller(t, m)
	c.expect("Test line")
	c.expect("login: ")
	c.send("alice\r")
	c.expect("alice\r\nPassword: ")
	c.send("wrong\r")
	c.expect("*****\r\n")
	c.expect("Login incorrect")
	c.expect("login: ")
	c.send("alice\r\n")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Welcome, alice.")
	c.expect("MOTD for alice\r\nLine closed Sunday\r\n")
	c.expect("[I]  Info")
	c.expect("Selection: ")

	// an unknown choice, the menu again, then the BBS by its name
	c.send("xyz\r")
	c.expect("Unknown selection")
	c.expect("Selection: ")
	c.send("?\r")
	c.expect("Main menu")
	c.expect("Selection: ")
	c.send("bbs\r")
	c.expect("Welcome to the BBS, alice")
	if h := <-hello; h != "\x00secret\x00alice\x00ansi/33600\x00" {
		t.Errorf("rlogin handshake %q", h)
	}
	c.send("Q")
	c.expect("Bye from the BBS")
	c.expect("Selection: ")

	// a cooked shell: ^C stops its command, not the session
	c.send("sh\r")
	c.expect("sh> ")
	c.send("sleep 30\r")
	c.expect("sleep 30")
	time.Sleep(300 * time.Millisecond)
	c.send("\x03")
	c.expect("sh> ")
	c.send("echo alive $MODEM_USER\r")
	c.expect("alive alice")
	c.send("exit\r")
	c.expect("Selection: ")

	c.send("Details\r")
	c.expect("args: alice 33600 5551000")
	c.send("typed\r")
	c.expect("read typed")
	c.expect("Goodbye")
	cmd.Wait()

	env, _ := os.ReadFile(envOut)
	for _, w := range []string{"MODEM_USER=alice", "MODEM_PASSWORD=secret", "MODEM_BAUD=33600",
		"MODEM_DIALED=5551000", "MODEM_CALLERID=5559999", "MODEM_SELECTION=info",
		"MODEM_AUTH=file", "MODEM_CHANNEL=Local/s@test", "MODEM_DATETIME=", `"user": "alice"`,
		`"password": "secret"`, `"dialed": "5551000"`} {
		if !bytes.Contains(env, []byte(w)) {
			t.Errorf("no %s in the environment and session file:\n%s", w, env)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "modemmenu-*.json")); len(matches) > 0 {
		t.Errorf("session files left: %v", matches)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	for _, w := range []string{`failed login user="alice"`, "login user=alice auth=file", "start bbs", "end info"} {
		if !bytes.Contains(log, []byte(w)) {
			t.Errorf("no %q in the log:\n%s", w, log)
		}
	}
	if bytes.Contains(log, []byte("secret")) {
		t.Errorf("the password in the log:\n%s", log)
	}
}

// the caller hanging up at the login ends modemmenu
func TestHangup(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	conf := filepath.Join(dir, "modemmenu.conf")
	os.WriteFile(conf, []byte("[general]\nlog = none\n[file]\npath = "+users+
		"\n[item ppp]\ncommand = exec true\n"), 0o600)
	m, s := testPty(t)
	cmd := exec.Command(bin, "-config", conf)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	c := newCaller(t, m)
	c.expect("login: ")
	c.send("ali")
	m.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("modemmenu is still running after the hangup")
	}
}
