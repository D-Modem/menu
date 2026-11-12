// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startServe: modemmenu built and serving conf's [server] listen at sock,
// stopped when the test ends
func startServe(t *testing.T, dir, conf, sock string) {
	t.Helper()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if _, _, err := openPty(); err != nil {
		t.Skip("no pty:", err)
	}
	srv := exec.Command(bin, "-config", lineMenu(conf), "-serve")
	out, err := os.Create(filepath.Join(dir, "serve.out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	srv.Stdout, srv.Stderr = out, out
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Process.Kill(); srv.Wait() })
	for i := 0; i < 100; i++ {
		// (the socket there, not a connection to it: that would be a
		// session, and hold one of max_sessions for a moment)
		if fi, err := os.Stat(sock); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(out.Name())
	t.Fatalf("%s: modemmenu -serve is not listening: %s", sock, b)
}

// line: a caller on a connection (or a pty's master), for a goroutine
// (no t.Fatal there): what came so far, and waits that fail with an
// error
type line struct {
	conn io.ReadWriteCloser
	ctl  net.Conn // a handed-over line's connection to the server
	mu   sync.Mutex
	got  strings.Builder
	done chan struct{}
}

// newLine: a caller on rw, what comes read as it comes
func newLine(rw io.ReadWriteCloser) *line {
	l := &line{conn: rw, done: make(chan struct{})}
	go func() {
		defer close(l.done)
		buf := make([]byte, 1024)
		for {
			n, err := rw.Read(buf)
			if n > 0 {
				// the terminal asked where its cursor is: 24x80
				if strings.Contains(string(buf[:n]), "\x1b[6n") {
					rw.Write([]byte("\x1b[24;80R"))
				}
				l.mu.Lock()
				l.got.Write(buf[:n])
				l.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return l
}

// dialLine: a caller connected to sock, the session header sent
func dialLine(sock, header string) (*line, error) {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	l := newLine(c)
	_, err = c.Write([]byte(header + "\n"))
	return l, err
}

// text: what came so far
func (l *line) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.got.String()
}

// expect: s comes within 15 s
func (l *line) expect(s string) error {
	until := time.Now().Add(15 * time.Second)
	for !strings.Contains(l.text(), s) {
		select {
		case <-l.done:
			if strings.Contains(l.text(), s) {
				return nil
			}
			return fmt.Errorf("waiting for %q: hung up; got %q", s, l.text())
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(until) {
			return fmt.Errorf("waiting for %q: timeout; got %q", s, l.text())
		}
	}
	return nil
}

// send: s to the session
func (l *line) send(s string) { l.conn.Write([]byte(s)) }

// fakeBBS: a BBS on a unix socket, as the bbs's [listen] with header =
// yes: each connection's header read, a greeting naming its user, then
// every line back with the user in front; how many connections at
// once at most, and how many in all
type fakeBBS struct {
	ln          net.Listener
	active, max atomic.Int32
	total       atomic.Int32
	mu          sync.Mutex
	users       map[string]int
}

// serveBBS: a fakeBBS listening at path
func serveBBS(t *testing.T, path string) *fakeBBS {
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBBS{ln: ln, users: map[string]int{}}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c)
		}
	}()
	return b
}

// serve: one connection, until the session hangs up
func (b *fakeBBS) serve(c net.Conn) {
	defer c.Close()
	b.total.Add(1)
	n := b.active.Add(1)
	defer b.active.Add(-1)
	for m := b.max.Load(); n > m && !b.max.CompareAndSwap(m, n); m = b.max.Load() {
	}
	r := bufio.NewReader(c)
	user := ""
	for {
		s, err := r.ReadString('\n')
		if err != nil {
			return
		}
		s = strings.TrimRight(s, "\r\n")
		if s == "" {
			break
		}
		if v, ok := strings.CutPrefix(s, "MODEM_USER="); ok {
			user = v
		}
	}
	b.mu.Lock()
	b.users[user]++
	b.mu.Unlock()
	fmt.Fprintf(c, "bbs: welcome %s\r\n", user)
	for {
		s, err := r.ReadString('\r')
		if err != nil {
			return
		}
		fmt.Fprintf(c, "%s:%s\n", user, s)
	}
}

// many callers at once on modemmenu -serve's unix socket, each its own
// session (header, login, menu) and each handed to a BBS on a unix socket
// by a connection of its own: all of them in the BBS together, no
// caller's bytes on another's line
func TestServeManyCallers(t *testing.T) {
	const callers = 8
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
`, filepath.Join(dir, "server.log"), filepath.Join(dir, "users"), sock, callers, bbsSock)), 0o600)
	bbs := serveBBS(t, bbsSock)
	startServe(t, dir, conf, sock)

	var (
		wg     sync.WaitGroup
		inBBS  sync.WaitGroup // every caller in the BBS: then they hang up
		errs   = make(chan error, callers)
		lines  = make([]*line, callers)
		failed atomic.Bool
	)
	inBBS.Add(callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived := false
			defer func() {
				if !arrived {
					failed.Store(true)
					inBBS.Done()
				}
			}()
			fail := func(err error) { errs <- fmt.Errorf("caller %d: %v", i, err) }
			l, err := dialLine(sock, fmt.Sprintf("DMODEM_CALLERID=555000%d\nDMODEM_RATE=33600/33600\n", i))
			if err != nil {
				fail(err)
				return
			}
			lines[i] = l
			defer l.conn.Close()
			user := fmt.Sprintf("user%d", i)
			for _, step := range []struct{ want, send string }{
				{"login: ", user + "\r"},
				{"Password: ", fmt.Sprintf("pw%d\r", i)},
				{"Selection: ", "b\r"},
				{"bbs: welcome " + user + "\r\n", fmt.Sprintf("ping %d\r", i)},
				{fmt.Sprintf("%s:ping %d\r", user, i), ""},
			} {
				if err := l.expect(step.want); err != nil {
					fail(err)
					return
				}
				if step.send != "" {
					l.send(step.send)
				}
			}
			arrived = true
			inBBS.Done()
			inBBS.Wait()
			// a last line, with everyone on: still only this caller's
			l.send(fmt.Sprintf("last %d\r", i))
			if err := l.expect(fmt.Sprintf("%s:last %d\r", user, i)); err != nil {
				fail(err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for i, l := range lines {
		if l == nil {
			continue
		}
		got := l.text()
		for j := 0; j < callers; j++ {
			if j != i && strings.Contains(got, fmt.Sprintf("user%d:", j)) {
				t.Errorf("caller %d got caller %d's bytes: %q", i, j, got)
			}
		}
	}
	if failed.Load() {
		return
	}
	if n := bbs.total.Load(); n != callers {
		t.Errorf("%d connections to the BBS, want %d", n, callers)
	}
	if n := bbs.max.Load(); n != callers {
		t.Errorf("%d callers in the BBS at once, want %d", n, callers)
	}
	bbs.mu.Lock()
	if len(bbs.users) != callers {
		t.Errorf("BBS users %v", bbs.users)
	}
	bbs.mu.Unlock()
}

// more callers than [server] max_sessions: those over it hear that the
// lines are busy, and a line that hangs up is free for the next
func TestServeBusy(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "menu.sock")
	conf := filepath.Join(dir, "server.conf")
	os.WriteFile(filepath.Join(dir, "users"), []byte("alice:{PLAIN}secret\n"), 0o600)
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
[file]
path = %s
[server]
listen = %s
max_sessions = 2
[item info]
key = I
label = Info
command = echo info
`, filepath.Join(dir, "users"), sock)), 0o600)
	startServe(t, dir, conf, sock)

	var held []*line
	for i := 0; i < 2; i++ {
		l, err := dialLine(sock, "")
		if err != nil {
			t.Fatal(err)
		}
		defer l.conn.Close()
		if err := l.expect("login: "); err != nil {
			t.Fatal(err)
		}
		held = append(held, l)
	}
	l, err := dialLine(sock, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.expect("All lines are busy"); err != nil {
		t.Fatal(err)
	}
	<-l.done
	// one hangs up: its line is free again
	held[0].conn.Close()
	<-held[0].done
	var last error
	for try := 0; try < 50; try++ {
		l, err := dialLine(sock, "")
		if err != nil {
			// still busy: the server told it so and hung up before the
			// header was written (broken pipe); try again
			last = err
			if l != nil {
				l.conn.Close()
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		last = l.expect("login: ")
		l.conn.Close()
		if last == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("after a hangup: %v", last)
}
