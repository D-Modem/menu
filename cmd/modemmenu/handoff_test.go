// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// handLine: a caller as app_dmodem's handoff = yes makes one: a pty, the
// modem on its master (the caller's end here), its terminal handed to
// modemmenu -serve at sock with the header
func handLine(sock, header string) (*line, error) {
	m, sl, err := openPty()
	if err != nil {
		return nil, err
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		m.Close()
		sl.Close()
		return nil, err
	}
	err = sendWithFD(c.(*net.UnixConn), []byte(header+"\n"), int(sl.Fd()))
	sl.Close()
	if err != nil {
		m.Close()
		c.Close()
		return nil, err
	}
	l := newLine(m)
	l.ctl = c
	return l, nil
}

// handoffBBS: a BBS that takes the line itself (the bbs's handoff
// listener): the header and the terminal, a greeting naming the user on
// the terminal, each line back with the user in front, until "quit":
// then "bye", and the line given back (the connection closed)
type handoffBBS struct {
	active, max, total atomic.Int32
	mu                 sync.Mutex
	failures           []string
}

// serveHandoffBBS: a handoffBBS listening at path
func serveHandoffBBS(t *testing.T, path string) *handoffBBS {
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	b := &handoffBBS{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c.(*net.UnixConn))
		}
	}()
	return b
}

// fail: a failure, for the test
func (b *handoffBBS) fail(f string, a ...any) {
	b.mu.Lock()
	b.failures = append(b.failures, fmt.Sprintf(f, a...))
	b.mu.Unlock()
}

// serve: one handed-over line, until "quit"
func (b *handoffBBS) serve(c *net.UnixConn) {
	defer c.Close()
	h, tty, err := readHeaderFD(c, 5*time.Second)
	if err != nil {
		b.fail("header: %v", err)
		return
	}
	if tty == nil {
		b.fail("%s: no line came with the header", h["MODEM_USER"])
		return
	}
	defer tty.Close()
	if _, err := unix.IoctlGetTermios(int(tty.Fd()), unix.TCGETS); err != nil {
		b.fail("%s: the line is not a terminal", h["MODEM_USER"])
		return
	}
	b.total.Add(1)
	n := b.active.Add(1)
	defer b.active.Add(-1)
	for m := b.max.Load(); n > m && !b.max.CompareAndSwap(m, n); m = b.max.Load() {
	}
	user := h["MODEM_USER"]
	fmt.Fprintf(tty, "bbs: welcome %s\r\n", user)
	r := bufio.NewReader(tty)
	for {
		s, err := r.ReadString('\r')
		if err != nil {
			return
		}
		if s == "quit\r" {
			fmt.Fprintf(tty, "bye %s\r\n", user)
			return
		}
		fmt.Fprintf(tty, "%s:%s\n", user, s)
	}
}

// the handoff both ways, many callers at once: each caller's line (a
// pty's terminal) handed to modemmenu -serve with the header, the session
// on it; an item with handoff = yes hands it on to the BBS, which has the
// terminal itself, and gives it back (the menu again); the caller hanging
// up ends the session and closes the connection
func TestServeHandoff(t *testing.T) {
	const callers = 8
	if _, _, err := openPty(); err != nil {
		t.Skip("no pty:", err)
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "menu.sock")
	bbsSock := filepath.Join(dir, "bbs.sock")
	var users strings.Builder
	for i := 0; i < callers; i++ {
		fmt.Fprintf(&users, "user%d:{PLAIN}pw%d\n", i, i)
	}
	os.WriteFile(filepath.Join(dir, "users"), []byte(users.String()), 0o600)
	conf := filepath.Join(dir, "server.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
[file]
path = %s
[server]
listen = %s
max_sessions = %d
[item bbs]
key = B
label = The BBS
mode = unix
path = %s
header = yes
handoff = yes
after = menu
[item tty]
key = T
label = The line
command = case "$(tty)" in /dev/pts/*) echo "on a terminal";; *) echo "not a terminal";; esac
after = menu
`, filepath.Join(dir, "server.log"), filepath.Join(dir, "users"), sock, callers, bbsSock)), 0o600)
	bbs := serveHandoffBBS(t, bbsSock)
	startServe(t, dir, conf, sock)

	var (
		wg    sync.WaitGroup
		inBBS sync.WaitGroup
		errs  = make(chan error, callers)
		lines = make([]*line, callers)
	)
	inBBS.Add(callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived := false
			defer func() {
				if !arrived {
					inBBS.Done()
				}
			}()
			fail := func(err error) { errs <- fmt.Errorf("caller %d: %v", i, err) }
			l, err := handLine(sock, fmt.Sprintf("DMODEM_CALLERID=555000%d\nDMODEM_RATE=33600/33600\n", i))
			if err != nil {
				fail(err)
				return
			}
			lines[i] = l
			defer l.conn.Close()
			defer l.ctl.Close()
			user := fmt.Sprintf("user%d", i)
			steps := func(st []struct{ want, send string }) bool {
				for _, step := range st {
					if err := l.expect(step.want); err != nil {
						fail(err)
						return false
					}
					if step.send != "" {
						l.send(step.send)
					}
				}
				return true
			}
			if !steps([]struct{ want, send string }{
				{"login: ", user + "\r"},
				{"Password: ", fmt.Sprintf("pw%d\r", i)},
				{"Selection: ", "t\r"},
				{"on a terminal", ""},
				{"Selection: ", "b\r"},
				{"bbs: welcome " + user + "\r\n", fmt.Sprintf("ping %d\r", i)},
				{fmt.Sprintf("%s:ping %d\r", user, i), ""},
			}) {
				return
			}
			arrived = true
			inBBS.Done()
			inBBS.Wait()
			// the BBS gives the line back: the menu again
			mark := len(l.text())
			l.send("quit\r")
			if !steps([]struct{ want, send string }{{"bye " + user + "\r\n", ""}}) {
				return
			}
			until := time.Now().Add(15 * time.Second)
			for !strings.Contains(l.text()[mark:], "Selection: ") {
				if time.Now().After(until) {
					fail(fmt.Errorf("no menu after the BBS: %q", l.text()[mark:]))
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			// the caller hangs up: the session ends, the connection
			// closes
			l.conn.Close()
			l.ctl.SetReadDeadline(time.Now().Add(15 * time.Second))
			if _, err := io.Copy(io.Discard, l.ctl); err != nil {
				fail(fmt.Errorf("the connection after the hangup: %v", err))
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for _, f := range bbs.failures {
		t.Error("bbs:", f)
	}
	for i, l := range lines {
		if l == nil {
			continue
		}
		for j := 0; j < callers; j++ {
			if j != i && strings.Contains(l.text(), fmt.Sprintf("user%d:", j)) {
				t.Errorf("caller %d got caller %d's bytes", i, j)
			}
		}
	}
	if n := bbs.total.Load(); n != callers {
		t.Errorf("%d lines handed to the BBS, want %d", n, callers)
	}
	if n := bbs.max.Load(); n != callers {
		t.Errorf("%d callers in the BBS at once, want %d", n, callers)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "server.log"))
	if n := strings.Count(string(log), "on the line handed over"); n != callers {
		t.Errorf("%d sessions on handed-over lines, want %d:\n%s", n, callers, log)
	}
}

// handoff = yes needs mode = unix and header = yes
func TestHandoffSettings(t *testing.T) {
	dir := t.TempDir()
	for _, item := range []string{
		"mode = tcp\nport = 23\nheader = yes",
		"mode = unix\npath = /x",
	} {
		conf := filepath.Join(dir, "m.conf")
		os.WriteFile(conf, []byte("[general]\nlog = none\nauth = file\n[file]\npath = /dev/null\n[item a]\nkey = A\nhandoff = yes\n"+item+"\n"), 0o600)
		if _, err := loadSettings(conf); err == nil || !strings.Contains(err.Error(), "handoff") {
			t.Errorf("%q: %v", item, err)
		}
	}
}

// a PPP session (a stand-in for pppd that would run for ever) on a
// handed-over line, past its time: modemmenu ends it at the deadline, the
// session ends and the connection closes, which hangs the call up
func TestHandoffTimeLimit(t *testing.T) {
	if _, _, err := openPty(); err != nil {
		t.Skip("no pty:", err)
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "menu.sock")
	os.WriteFile(filepath.Join(dir, "users"), []byte("alice:{PLAIN}secret\n"), 0o600)
	conf := filepath.Join(dir, "server.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
[file]
path = %s
[server]
listen = %s
[item ppp]
key = P
label = PPP
command = echo "pppd up"; exec sleep 1000
`, filepath.Join(dir, "server.log"), filepath.Join(dir, "users"), sock)), 0o600)
	startServe(t, dir, conf, sock)

	// the call has 4 seconds (an upstream's MODEM_SECONDS_LEFT)
	l, err := handLine(sock, "DMODEM_CALLERID=5550000\nMODEM_SECONDS_LEFT=4\n")
	if err != nil {
		t.Fatal(err)
	}
	defer l.conn.Close()
	defer l.ctl.Close()
	for _, step := range []struct{ want, send string }{
		{"login: ", "alice\r"},
		{"Password: ", "secret\r"},
		{"Selection: ", "p\r"},
		{"pppd up", ""},
	} {
		if err := l.expect(step.want); err != nil {
			t.Fatal(err)
		}
		if step.send != "" {
			l.send(step.send)
		}
	}
	start := time.Now()
	l.ctl.SetReadDeadline(time.Now().Add(30 * time.Second))
	if _, err := io.Copy(io.Discard, l.ctl); err != nil {
		t.Fatalf("the connection still open past the time limit: %v", err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("hung up %v after PPP started", d)
	}
	// the server logs the session's end just after the line closes
	var log []byte
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if log, _ = os.ReadFile(filepath.Join(dir, "server.log")); strings.Contains(string(log), "ended") {
			break
		}
	}
	if !strings.Contains(string(log), "on the line handed over") || !strings.Contains(string(log), "ended") {
		t.Errorf("log:\n%s", log)
	}
}
