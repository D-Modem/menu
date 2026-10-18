// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The session header: the details of a call, ahead of the caller's bytes
// on a socket (which has no environment to carry them).  KEY=value lines
// ending in an empty line; app_dmodem (header = yes) sends DMODEM_* keys,
// a modemmenu item (header = yes) MODEM_* keys.

const (
	headerMaxLines = 64
	headerMaxBytes = 8192
)

// writeHeader: the header of the session, MODEM_* (with the password when
// withPassword)
func writeHeader(w io.Writer, s *session, withPassword bool) error {
	var b strings.Builder
	for _, kv := range s.environ(withPassword) {
		k, v, _ := strings.Cut(kv, "=")
		b.WriteString(k + "=" + headerValue(v) + "\n")
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// headerValue: a value on one line
func headerValue(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return ' '
		}
		return r
	}, v)
}

// readHeader from r one byte at a time (what follows it is the caller's),
// until the empty line or the deadline
func readHeader(conn net.Conn, timeout time.Duration) (map[string]string, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})
	h := map[string]string{}
	var line []byte
	total := 0
	b := make([]byte, 1)
	for {
		if _, err := conn.Read(b); err != nil {
			return nil, fmt.Errorf("header: %v", err)
		}
		if total++; total > headerMaxBytes {
			return nil, errors.New("header: too long")
		}
		if b[0] != '\n' {
			line = append(line, b[0])
			continue
		}
		s := strings.TrimSuffix(string(line), "\r")
		line = line[:0]
		if s == "" {
			return h, nil
		}
		if len(h) >= headerMaxLines {
			return nil, errors.New("header: too many lines")
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("header: no = in %q", s)
		}
		h[strings.ToUpper(strings.TrimSpace(k))] = v
	}
}

// headerEnv: the environment of a session from a header: the call's
// details as app_dmodem's DMODEM_*, and with trust the caller an
// upstream menu logged in already
func headerEnv(h map[string]string, trust bool) []string {
	call := map[string]string{
		"DMODEM_CHANNEL": "MODEM_CHANNEL", "DMODEM_PROFILE": "MODEM_PROFILE",
		"DMODEM_RATE": "MODEM_RATE", "DMODEM_CALLERID": "MODEM_CALLERID",
		"DMODEM_DIALED": "MODEM_DIALED", "DMODEM_CONNECTED": "MODEM_EPOCH",
	}
	var env []string
	for d, m := range call {
		if v, ok := h[d]; ok {
			env = append(env, d+"="+v)
		} else if v, ok := h[m]; ok {
			env = append(env, d+"="+v)
		}
	}
	// the time an upstream menu leaves the caller (more exact in seconds),
	// a limit here too, trusted or not: it can only shorten the call
	if v := h["MODEM_SECONDS_LEFT"]; v != "" {
		env = append(env, "MODEMMENU_UPSTREAM_SECONDS_LEFT="+v)
	} else if v, err := strconv.Atoi(h["MODEM_TIME_LEFT"]); err == nil {
		env = append(env, "MODEMMENU_UPSTREAM_SECONDS_LEFT="+strconv.Itoa(v*60))
	}
	// the terminal, as the upstream menu found it: not asked again
	for _, k := range []string{"TERMINAL", "TERM", "TERM_NAME", "ROWS", "COLS"} {
		if v := h["MODEM_"+k]; v != "" {
			env = append(env, "MODEMMENU_UPSTREAM_"+k+"="+v)
		}
	}
	if trust && h["MODEM_USER"] != "" {
		env = append(env, "MODEMMENU_UPSTREAM_USER="+h["MODEM_USER"],
			"MODEMMENU_UPSTREAM_PASSWORD="+h["MODEM_PASSWORD"],
			"MODEMMENU_UPSTREAM_AUTH="+h["MODEM_AUTH"])
	}
	return env
}

// serve: sessions for the connections to listen, each modemmenu on a pty
// of its own as under app_dmodem
func serve(s *settings, config string) int {
	srv := s.server
	ln, err := listen(srv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	logf("serving on %s", srv.listen)
	var authLn net.Listener
	if s.authd != nil {
		if authLn, err = listenAuth(s.authd); err != nil {
			ln.Close()
			fmt.Fprintln(os.Stderr, "modemmenu:", err)
			return 1
		}
		go serveAuth(s, authLn)
	}

	var (
		mu       sync.Mutex
		sessions = map[*os.File]bool{} // the pty masters
		wg       sync.WaitGroup
	)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		sig := <-ch
		logf("%v: stopping", sig)
		ln.Close()
		if authLn != nil {
			authLn.Close()
		}
	}()

	slots := make(chan struct{}, srv.maxSessions)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			logf("accept: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			logf("%s: %d sessions already, refused", peer(conn), srv.maxSessions)
			io.WriteString(conn, "\r\nAll lines are busy, call again later.\r\n")
			conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			serveConn(conn, srv, self, config, func(m *os.File, add bool) {
				mu.Lock()
				if add {
					sessions[m] = true
				} else {
					delete(sessions, m)
				}
				mu.Unlock()
			})
		}()
	}
	// hang up every session: closing a master hangs up its pty
	mu.Lock()
	for m := range sessions {
		m.Close()
	}
	mu.Unlock()
	wg.Wait()
	if strings.HasPrefix(srv.listen, "/") {
		os.Remove(srv.listen)
	}
	return 0
}

// listen on a unix socket (a path) or tcp:host:port
func listen(srv *serverSettings) (net.Listener, error) {
	if addr, ok := strings.CutPrefix(srv.listen, "tcp:"); ok {
		return net.Listen("tcp", addr)
	}
	path := srv.listen
	// a socket left by a modemmenu that died; not anything else
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
		if c, err := net.Dial("unix", path); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s: another modemmenu is serving there", path)
		}
		os.Remove(path)
	}
	old := unix.Umask(0o077)
	ln, err := net.Listen("unix", path)
	unix.Umask(old)
	if err != nil {
		return nil, err
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(true)
	if srv.socketGroup != "" {
		g, err := user.LookupGroup(srv.socketGroup)
		if err != nil {
			ln.Close()
			return nil, fmt.Errorf("[server] socket_group: %v", err)
		}
		gid, _ := strconv.Atoi(g.Gid)
		if err := os.Chown(path, -1, gid); err != nil {
			ln.Close()
			return nil, err
		}
	}
	if err := os.Chmod(path, srv.socketMode); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// peer: who connected, for the log
func peer(conn net.Conn) string {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return conn.RemoteAddr().String()
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return "unix"
	}
	var cred *unix.Ucred
	raw.Control(func(fd uintptr) {
		cred, _ = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if cred == nil {
		return "unix"
	}
	return fmt.Sprintf("pid %d uid %d", cred.Pid, cred.Uid)
}

// serveConn: the header, then modemmenu on a new pty, its bytes to and from
// the connection until either ends
func serveConn(conn net.Conn, srv *serverSettings, self, config string, track func(*os.File, bool)) {
	defer conn.Close()
	who := peer(conn)
	env := os.Environ()
	keep := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "DMODEM_") && !strings.HasPrefix(e, "MODEM") {
			keep = append(keep, e)
		}
	}
	env = keep
	if srv.header {
		h, err := readHeader(conn, srv.headerTimeout)
		if err != nil {
			logf("%s: %v", who, err)
			return
		}
		env = append(env, headerEnv(h, srv.trustUser)...)
	}

	master, slave, err := openPty()
	if err != nil {
		logf("%s: %v", who, err)
		return
	}
	cmd := exec.Command(self, "-config", config, "-upstream="+strconv.FormatBool(srv.trustUser))
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	err = cmd.Start()
	slave.Close()
	if err != nil {
		master.Close()
		logf("%s: %v", who, err)
		return
	}
	logf("%s: session pid %d", who, cmd.Process.Pid)
	track(master, true)

	var once sync.Once
	hangup := func() { once.Do(func() { master.Close() }) }
	// the caller to the session; the caller gone hangs it up
	go func() {
		io.Copy(master, conn)
		hangup()
	}()
	// the session to the caller, until the pty is hung up or the session
	// has ended and its last bytes are out (EIO)
	io.Copy(conn, master)
	cmd.Wait()
	hangup()
	track(master, false)
	logf("%s: session pid %d ended", who, cmd.Process.Pid)
}

// openPty: a new pty, raw as app_dmodem makes it, master and slave
func openPty() (*os.File, *os.File, error) {
	// nonblocking, so that the runtime polls it: a Close then ends a Read
	// in progress (and hangs up the pty) instead of waiting for it
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	m := os.NewFile(uintptr(fd), "/dev/ptmx")
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		return nil, nil, err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	s, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	if tio, err := unix.IoctlGetTermios(int(s.Fd()), unix.TCGETS); err == nil {
		makeRaw(tio)
		unix.IoctlSetTermios(int(s.Fd()), unix.TCSETS, tio)
	}
	return m, s, nil
}

// upstream: the caller as an upstream menu logged them in, from the
// environment of a session of modemmenu -serve with trust_user (trusted
// only then); the variables are removed either way, so that no selection
// sees them
func upstream(sess *session, trusted bool) bool {
	if secs, err := strconv.Atoi(os.Getenv("MODEMMENU_UPSTREAM_SECONDS_LEFT")); err == nil && secs >= 0 {
		sess.upstreamDeadline = time.Now().Add(time.Duration(secs) * time.Second)
	}
	os.Unsetenv("MODEMMENU_UPSTREAM_SECONDS_LEFT")
	u := os.Getenv("MODEMMENU_UPSTREAM_USER")
	pw := os.Getenv("MODEMMENU_UPSTREAM_PASSWORD")
	auth := os.Getenv("MODEMMENU_UPSTREAM_AUTH")
	for _, k := range []string{"MODEMMENU_UPSTREAM_USER", "MODEMMENU_UPSTREAM_PASSWORD", "MODEMMENU_UPSTREAM_AUTH"} {
		os.Unsetenv(k)
	}
	if !trusted || u == "" {
		return false
	}
	sess.User, sess.Password, sess.Auth = u, pw, "upstream"
	if auth != "" {
		sess.Auth = "upstream:" + auth
	}
	return true
}
