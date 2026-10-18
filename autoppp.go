// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// AutoPPP: a caller's dialer that starts PPP at once (Windows' Dial-Up
// Networking and most others) sends a PPP frame where the login prompt
// wants a name.  modemmenu sees it, and hands the line to pppd (the
// [autoppp] command), whose modemmenu.so plugin asks for the PAP login
// on an auth socket of this call's own ({auth_socket}): the backends
// check it as at the prompt, then the access rules of [autoppp] item, then
// the time limits, whose time left becomes pppd's maxconnect.

// errAutoPPP: a PPP frame came instead of a name
var errAutoPPP = errors.New("PPP frame at the login prompt")

// autopppSettings: [autoppp]
type autopppSettings struct {
	enabled bool
	command string
	item    string // whose access rules a PPP login must pass ("": none)
}

// loadAutoPPP: [autoppp]; enabled needs a command
func loadAutoPPP(sec *section) (autopppSettings, error) {
	var err error
	a := autopppSettings{command: sec.get("command", ""), item: sec.get("item", "")}
	if a.enabled, err = sec.bool("enabled", false); err != nil {
		return a, err
	}
	if a.enabled && a.command == "" {
		return a, fmt.Errorf("[autoppp] enabled needs a command (pppd)")
	}
	return a, nil
}

// pppFrame: whether what starts with the flag 0x7E is a PPP frame: the
// address and control (0xFF 0x03, the 0x03 escaped as 0x7D 0x23), the
// address escaped (0x7D 0xDF), or both compressed away (LCP's 0xC0 0x21)
func pppFrame(b []byte) bool {
	if len(b) < 3 || b[0] != 0x7E {
		return false
	}
	switch {
	case b[1] == 0xFF && (b[2] == 0x03 || b[2] == 0x7D):
		return true
	case b[1] == 0x7D && b[2] == 0xDF:
		return true
	case b[1] == 0xC0 && b[2] == 0x21:
		return true
	}
	return false
}

// sniffPPP: at the login prompt, whether a PPP frame comes; what is not
// one is put back for the name
func sniffPPP(t *term, deadline time.Time) (bool, error) {
	c, err := t.readByte(time.Until(deadline))
	if err != nil {
		return false, err
	}
	got := []byte{c}
	if c == 0x7E {
		// the rest of the frame's start, which follows at once
		for len(got) < 3 {
			c, err := t.readByte(time.Second)
			if err != nil {
				break
			}
			got = append(got, c)
		}
		if pppFrame(got) {
			return true, nil
		}
	}
	t.unread(got...)
	return false, nil
}

// pppAuth: the auth socket of a call's pppd, until close
type pppAuth struct {
	s     *settings
	sess  *session
	dir   string
	ln    net.Listener
	mu    sync.Mutex
	fails int
	lim   *limiter      // set by the login
	login chan struct{} // closed by the login
}

// startPPPAuth: the socket, in a directory of its own
func startPPPAuth(s *settings, sess *session) (*pppAuth, error) {
	dir, err := os.MkdirTemp("", "modemmenu-ppp-")
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", filepath.Join(dir, "auth.sock"))
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	a := &pppAuth{s: s, sess: sess, dir: dir, ln: ln, login: make(chan struct{})}
	go a.serve()
	return a, nil
}

// path: the auth socket's path, for {auth_socket}
func (a *pppAuth) path() string { return filepath.Join(a.dir, "auth.sock") }

// close: the socket closed and its directory removed
func (a *pppAuth) close() {
	a.ln.Close()
	os.RemoveAll(a.dir)
}

// serve: each connection to the socket answered, until close
func (a *pppAuth) serve() {
	for {
		conn, err := a.ln.Accept()
		if err != nil {
			return
		}
		go a.answer(conn)
	}
}

// pppReply: the auth socket's answer to the plugin
type pppReply struct {
	OK          bool   `json:"ok"`
	Auth        string `json:"auth,omitempty"`
	SecondsLeft int    `json:"seconds_left"` // -1: no limit
	Error       string `json:"error,omitempty"`
}

// answer one PAP login: the backends, the [autoppp] item's rules, the
// time limits; one login a call, attempts tries
func (a *pppAuth) answer(conn net.Conn) {
	defer conn.Close()
	reply := func(r pppReply) {
		b, _ := json.Marshal(r)
		conn.Write(append(b, '\n'))
	}
	conn.SetDeadline(time.Now().Add(time.Minute))
	line, err := bufio.NewReaderSize(io.LimitReader(conn, maxAuthRequest), maxAuthRequest).ReadSlice('\n')
	var req authRequest
	if err != nil || json.Unmarshal(line, &req) != nil || req.User == "" {
		reply(pppReply{Error: "bad request"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, sess := a.s, a.sess
	if a.lim != nil {
		reply(pppReply{Error: "logged in already"})
		return
	}
	if a.fails >= s.attempts {
		reply(pppReply{Error: "too many attempts"})
		return
	}
	var auth string
	if s.userPattern.MatchString(req.User) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		auth, err = authenticate(ctx, s.backends, req.User, req.Password, logf)
		cancel()
	} else {
		burn(req.Password)
		err = errNoUser
	}
	if err != nil {
		a.fails++
		logf("failed PPP login user=%q (%v) %v", req.User, err, sess)
		time.Sleep(s.failDelay)
		reply(pppReply{Error: "denied"})
		return
	}
	sess.User, sess.Password, sess.Auth = req.User, req.Password, auth
	if name := s.autoppp.item; name != "" {
		if it := s.selects(strings.ToUpper(name)); it == nil || !s.allows(it, sess) {
			a.fails++
			logf("PPP login user=%s: not allowed [item %s] %v", req.User, name, sess)
			sess.User, sess.Password, sess.Auth = "", "", ""
			time.Sleep(s.failDelay)
			reply(pppReply{Error: "denied"})
			return
		}
	}
	lim, err := newLimiter(s, sess, time.Now())
	if err != nil {
		logf("PPP login user=%s: no time left today", req.User)
		sess.User, sess.Password, sess.Auth = "", "", ""
		reply(pppReply{Error: "no time left"})
		return
	}
	a.lim = lim
	setAtExit(lim.done)
	if s.hasCallers() {
		if err := recordCaller(s.limits.stateDir, sess, time.Now()); err != nil {
			logf("callers: %v", err)
		}
	}
	logf("login user=%s auth=%s via PPP %v", sess.User, auth, sess)
	secs := -1
	if left, ok := lim.left(time.Now()); ok {
		secs = int(left / time.Second)
	}
	close(a.login)
	reply(pppReply{OK: true, Auth: auth, SecondsLeft: secs})
}

// autoPPP: the [autoppp] command on the line, with the call's auth
// socket, until it ends; the exit status
func autoPPP(t *term, s *settings, sess *session) int {
	logf("PPP frame at the login prompt: %s %v", s.autoppp.command, sess)
	a, err := startPPPAuth(s, sess)
	if err != nil {
		logf("autoppp: %v", err)
		return 1
	}
	defer a.close()

	v := sess.vars(false)
	v["auth_socket"] = a.path()
	cmd := exec.Command("/bin/sh", "-c", expand(s.autoppp.command, v, shellQuote))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stdout
	env := make([]string, 0, 32)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "MODEM_") {
			env = append(env, e)
		}
	}
	env = append(env, sess.environ(false)...)
	env = append(env, "MODEMMENU_AUTH_SOCKET="+a.path())
	cmd.Env = env
	before := t.setMode("raw")
	defer t.restoreMode(before)
	if err := cmd.Start(); err != nil {
		logf("autoppp: %v", err)
		return 1
	}
	running(cmd.Process)

	// once logged in: the time limit (pppd's maxconnect ends the link
	// then; a hangup soon after, should it not)
	done := make(chan struct{})
	go func() {
		select {
		case <-a.login:
		case <-done:
			return
		}
		if a.lim.deadline.IsZero() {
			return
		}
		select {
		case <-time.After(time.Until(a.lim.deadline) + 10*time.Second):
			logf("autoppp user=%s: time limit", sess.User)
			cmd.Process.Signal(syscall.SIGHUP)
			time.AfterFunc(10*time.Second, func() { cmd.Process.Kill() })
		case <-done:
		}
	}()
	err = cmd.Wait()
	close(done)
	running(nil)
	unix.IoctlSetPointerInt(t.in, unix.TIOCSPGRP, unix.Getpgrp())
	a.mu.Lock()
	lim := a.lim
	a.mu.Unlock()
	if lim != nil {
		lim.done()
		logf("logout user=%s: PPP ended (%v)", sess.User, describe(err))
	} else {
		logf("PPP ended without a login (%v) %v", describe(err), sess)
	}
	return 0
}
