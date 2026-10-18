// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// launch the selection: its error, or nil when it ended by itself;
// errTimeLimit when deadline (if not zero) ended it
func launch(t *term, cfg *settings, it *menuItem, s *session, deadline time.Time, logf func(string, ...any)) error {
	// each time_warning before the deadline (those still ahead), on the
	// line in the middle of what the selection says
	for _, warn := range cfg.limits.warnings {
		if !it.timeWarning || deadline.IsZero() || time.Until(deadline) <= warn {
			continue
		}
		timer := time.AfterFunc(time.Until(deadline)-warn, func() {
			left := time.Until(deadline)
			v := s.vars(false)
			v["time_left"] = strconv.Itoa(int((left + 30*time.Second) / time.Minute))
			v["seconds_left"] = strconv.Itoa(int((left + 500*time.Millisecond) / time.Second))
			t.write(expand(cfg.limits.warningMessage, v, nil))
			logf("%s: time warning user=%s left=%v", it.name, s.User, left.Round(time.Second))
		})
		defer timer.Stop()
	}
	switch it.mode {
	case "passwd":
		return changePasswordItem(t, cfg, it, s, deadline)
	case "callers":
		return callersItem(t, cfg, it)
	case "exec":
		return runExec(t, cfg, it, s, deadline, logf)
	default:
		return runTCP(t, it, s, deadline)
	}
}

// runExec: the command under /bin/sh -c on the caller's line, the
// session in MODEM_* of its environment, {placeholders} of the command
// as quoted shell words
func runExec(t *term, cfg *settings, it *menuItem, s *session, deadline time.Time, logf func(string, ...any)) error {
	v := s.vars(it.password)
	env := make([]string, 0, 32)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "MODEM_") {
			env = append(env, e)
		}
	}
	env = append(env, s.environ(it.password)...)
	if s.Term != "" {
		// the terminal as detected
		env = setEnv(env, "TERM", s.Term)
	}

	uid, gid := -1, -1
	var cred *syscall.Credential
	var home string
	if it.runAs != "" {
		name := expand(it.runAs, v, nil)
		u, err := user.Lookup(name)
		if err != nil {
			return fmt.Errorf("run_as %s: %v", name, err)
		}
		if cred, err = credential(u); err != nil {
			return fmt.Errorf("run_as %s: %v", name, err)
		}
		uid, gid = int(cred.Uid), int(cred.Gid)
		home = u.HomeDir
		env = setEnv(env, "HOME", u.HomeDir)
		env = setEnv(env, "USER", u.Username)
		env = setEnv(env, "LOGNAME", u.Username)
		if sh := loginShell(u.Username); sh != "" {
			env = setEnv(env, "SHELL", sh)
		}
	}
	// the drop files of a door: the node, their directory and the files,
	// for {node} and {dropfile_dir} in the command too
	if len(it.dropfiles) > 0 {
		df, err := writeDropfiles(cfg, it, s, deadline, uid, gid)
		if err != nil {
			return fmt.Errorf("drop files: %v", err)
		}
		defer df.close()
		v["node"], v["dropfile_dir"] = strconv.Itoa(df.node), df.dir
		// the first file's path, as BBSs pass it (Synchronet's %f)
		v["dropfile"] = df.files[0]
		env = append(env, "MODEM_NODE="+v["node"], "MODEM_DROPFILE_DIR="+df.dir,
			"MODEM_DROPFILE="+df.files[0])
	}

	cmd := exec.Command("/bin/sh", "-c", expand(it.command, v, shellQuote))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stdout
	if cred != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
		cmd.Dir = home
	}
	if it.sessionFile {
		path, err := s.writeFile(it.password, uid, gid)
		if err != nil {
			return fmt.Errorf("session file: %v", err)
		}
		defer os.Remove(path)
		env = append(env, "MODEM_SESSION_FILE="+path)
	}
	cmd.Env = env

	before := t.setMode(it.tty)
	defer t.restoreMode(before)
	if err := cmd.Start(); err != nil {
		return err
	}
	running(cmd.Process)
	// at the deadline: hung up, as when the caller hangs up, then killed
	var timedOut atomic.Bool
	if !deadline.IsZero() {
		p := cmd.Process
		timer := time.AfterFunc(time.Until(deadline), func() {
			timedOut.Store(true)
			p.Signal(syscall.SIGHUP)
			time.AfterFunc(10*time.Second, func() { p.Kill() })
		})
		defer timer.Stop()
	}
	err := cmd.Wait()
	running(nil)
	if timedOut.Load() {
		return errTimeLimit
	}
	// the line's foreground is ours again, whatever a shell did with it
	unix.IoctlSetPointerInt(t.in, unix.TIOCSPGRP, unix.Getpgrp())
	if ee, ok := err.(*exec.ExitError); ok {
		logf("%s: %s", it.name, ee.ProcessState)
		return nil
	}
	return err
}

func credential(u *user.User) (*syscall.Credential, error) {
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	c := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	if ids, err := u.GroupIds(); err == nil {
		for _, id := range ids {
			if g, err := strconv.ParseUint(id, 10, 32); err == nil {
				c.Groups = append(c.Groups, uint32(g))
			}
		}
	}
	return c, nil
}

// loginShell of a user in /etc/passwd
func loginShell(name string) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), ":")
		if len(p) == 7 && p[0] == name {
			return p[6]
		}
	}
	return ""
}

func setEnv(env []string, k, v string) []string {
	for i, e := range env {
		if strings.HasPrefix(e, k+"=") {
			env[i] = k + "=" + v
			return env
		}
	}
	return append(env, k+"="+v)
}

// the program of the selection, for the signal handler
var (
	childMu sync.Mutex
	child   *os.Process
)

func running(p *os.Process) {
	childMu.Lock()
	child = p
	childMu.Unlock()
}

// runTCP: the caller's line to a server, by TCP or a unix socket: raw,
// telnet or rlogin
func runTCP(t *term, it *menuItem, s *session, deadline time.Time) error {
	v := s.vars(it.password)
	network, addr := "tcp", net.JoinHostPort(it.host, strconv.Itoa(it.port))
	if it.path != "" {
		network, addr = "unix", it.path
	}
	conn, err := net.DialTimeout(network, addr, it.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if it.header {
		// the session ahead of the protocol: for a modemmenu -serve, or
		// any program that reads it
		if err := writeHeader(conn, s, it.password); err != nil {
			return err
		}
	}

	var tn *telnet
	switch it.mode {
	case "rlogin":
		// RFC 1282: \0 client-user \0 server-user \0 terminal/speed \0;
		// BBSs (Synchronet, Mystic) take the password as the client
		// user and log the caller in at once
		hello := "\x00" + expand(it.rloginLocal, v, nil) + "\x00" +
			expand(it.rloginUser, v, nil) + "\x00" +
			expand(it.rloginTerm, v, nil) + "\x00"
		conn.SetDeadline(time.Now().Add(it.timeout))
		if _, err := io.WriteString(conn, hello); err != nil {
			return err
		}
		var ack [1]byte
		if _, err := io.ReadFull(conn, ack[:]); err != nil {
			return fmt.Errorf("rlogin: %v", err)
		}
		conn.SetDeadline(time.Time{})
		if ack[0] != 0 {
			// not the ack: the server's first byte of output
			t.writeRaw(string(ack[:]))
		}
	case "telnet":
		tn = &telnet{conn: conn, term: expand(it.terminal, v, nil),
			local: map[byte]bool{}, remote: map[byte]bool{}}
	}
	// at the deadline the connection's reads and writes fail: the end
	conn.SetDeadline(deadline)
	timeUp := func(err error) error {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return errTimeLimit
		}
		return err
	}
	if it.send != "" {
		b := []byte(expand(unescape(it.send), v, nil))
		if tn != nil {
			b = tn.encode(b)
		}
		if _, err := conn.Write(b); err != nil {
			return err
		}
	}

	// what the caller sent that the menu read and did not take
	if len(t.pending) > 0 {
		b := t.pending
		t.pending = nil
		if tn != nil {
			b = tn.encode(b)
		}
		if _, err := conn.Write(b); err != nil {
			return timeUp(err)
		}
	}

	// the server to the caller
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				out := buf[:n]
				if tn != nil {
					out = tn.decode(out)
				}
				if werr := t.writeRaw(string(out)); werr != nil {
					done <- werr
					return
				}
			}
			if err != nil {
				done <- nil
				return
			}
		}
	}()

	// the caller to the server, until either goes away; stdin is polled
	// so that nothing is read from it once the server is gone
	buf := make([]byte, 4096)
	for {
		select {
		case err := <-done:
			return timeUp(err)
		default:
		}
		ok, err := pollOne(t.in, unix.POLLIN, 200*time.Millisecond)
		if err != nil {
			return errHangup
		}
		if !ok {
			continue
		}
		n, err := unix.Read(t.in, buf)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if n <= 0 || err != nil {
			return errHangup
		}
		b := buf[:n]
		if tn != nil {
			b = tn.encode(b)
		}
		if _, err := tn.writeTo(conn, b); err != nil {
			<-done
			return timeUp(nil)
		}
	}
}

// telnet: just enough of a telnet client for a BBS: binary, suppress go
// ahead and the server's echo, and the terminal type
type telnet struct {
	mu     sync.Mutex
	conn   io.Writer
	term   string
	state  int
	cmd    byte
	sb     []byte
	local  map[byte]bool // options we do
	remote map[byte]bool // options the server does
}

const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240

	optBinary = 0
	optEcho   = 1
	optSGA    = 3
	optTType  = 24
)

func (tn *telnet) writeTo(w io.Writer, b []byte) (int, error) {
	if tn == nil {
		return w.Write(b)
	}
	tn.mu.Lock()
	defer tn.mu.Unlock()
	return w.Write(b)
}

func (tn *telnet) send(b ...byte) {
	tn.writeTo(tn.conn, b)
}

// encode the caller's bytes: IAC doubled, CR as CR NUL unless binary
func (tn *telnet) encode(b []byte) []byte {
	tn.mu.Lock()
	bin := tn.local[optBinary]
	tn.mu.Unlock()
	out := make([]byte, 0, len(b)+8)
	for _, c := range b {
		out = append(out, c)
		if c == iac {
			out = append(out, iac)
		} else if c == '\r' && !bin {
			out = append(out, 0)
		}
	}
	return out
}

// decode the server's bytes: its data, the commands answered
func (tn *telnet) decode(b []byte) []byte {
	out := b[:0:0]
	for _, c := range b {
		switch tn.state {
		case 0:
			if c == iac {
				tn.state = 1
			} else {
				out = append(out, c)
			}
		case 1:
			switch c {
			case iac:
				out = append(out, iac)
				tn.state = 0
			case will, wont, do, dont:
				tn.cmd, tn.state = c, 2
			case sb:
				tn.sb, tn.state = tn.sb[:0], 3
			default:
				tn.state = 0
			}
		case 2:
			tn.option(tn.cmd, c)
			tn.state = 0
		case 3:
			if c == iac {
				tn.state = 4
			} else if len(tn.sb) < 256 {
				tn.sb = append(tn.sb, c)
			}
		case 4:
			if c == se {
				tn.subneg()
				tn.state = 0
			} else {
				if c == iac && len(tn.sb) < 256 {
					tn.sb = append(tn.sb, iac)
				}
				tn.state = 3
			}
		}
	}
	return out
}

func (tn *telnet) option(cmd, opt byte) {
	switch cmd {
	case will:
		ok := opt == optEcho || opt == optSGA || opt == optBinary
		if ok && !tn.remote[opt] {
			tn.remote[opt] = true
			tn.send(iac, do, opt)
		} else if !ok {
			tn.send(iac, dont, opt)
		}
	case wont:
		if tn.remote[opt] {
			tn.remote[opt] = false
			tn.send(iac, dont, opt)
		}
	case do:
		ok := opt == optSGA || opt == optBinary || opt == optTType
		if ok && !tn.local[opt] {
			tn.mu.Lock()
			tn.local[opt] = true
			tn.mu.Unlock()
			tn.send(iac, will, opt)
		} else if !ok {
			tn.send(iac, wont, opt)
		}
	case dont:
		if tn.local[opt] {
			tn.mu.Lock()
			tn.local[opt] = false
			tn.mu.Unlock()
			tn.send(iac, wont, opt)
		}
	}
}

func (tn *telnet) subneg() {
	// TTYPE SEND: TTYPE IS <terminal>
	if len(tn.sb) >= 2 && tn.sb[0] == optTType && tn.sb[1] == 1 {
		msg := []byte{iac, sb, optTType, 0}
		msg = append(msg, []byte(tn.term)...)
		tn.send(append(msg, iac, se)...)
	}
}
