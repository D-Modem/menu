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
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// launch the selection: its error, or nil when it ended by itself
func launch(t *term, it *menuItem, s *session, logf func(string, ...any)) error {
	switch it.mode {
	case "exec":
		return runExec(t, it, s, logf)
	default:
		return runTCP(t, it, s)
	}
}

// runExec: the command under /bin/sh -c on the caller's line, the
// session in MODEM_* of its environment, {placeholders} of the command
// as quoted shell words
func runExec(t *term, it *menuItem, s *session, logf func(string, ...any)) error {
	v := s.vars(it.password)
	cmd := exec.Command("/bin/sh", "-c", expand(it.command, v, shellQuote))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stdout

	env := make([]string, 0, 32)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "MODEM_") {
			env = append(env, e)
		}
	}
	env = append(env, s.environ(it.password)...)

	uid, gid := -1, -1
	if it.runAs != "" {
		name := expand(it.runAs, v, nil)
		u, err := user.Lookup(name)
		if err != nil {
			return fmt.Errorf("run_as %s: %v", name, err)
		}
		cred, err := credential(u)
		if err != nil {
			return fmt.Errorf("run_as %s: %v", name, err)
		}
		uid, gid = int(cred.Uid), int(cred.Gid)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
		cmd.Dir = u.HomeDir
		env = setEnv(env, "HOME", u.HomeDir)
		env = setEnv(env, "USER", u.Username)
		env = setEnv(env, "LOGNAME", u.Username)
		if sh := loginShell(u.Username); sh != "" {
			env = setEnv(env, "SHELL", sh)
		}
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
	err := cmd.Wait()
	running(nil)
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

// runTCP: the caller's line to a server: raw TCP, telnet or rlogin
func runTCP(t *term, it *menuItem, s *session) error {
	v := s.vars(it.password)
	addr := net.JoinHostPort(it.host, strconv.Itoa(it.port))
	conn, err := net.DialTimeout("tcp", addr, it.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

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
			t.write(string(ack[:]))
		}
	case "telnet":
		tn = &telnet{conn: conn, term: expand(it.terminal, v, nil),
			local: map[byte]bool{}, remote: map[byte]bool{}}
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
				if werr := t.write(string(out)); werr != nil {
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
			return err
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
			return nil
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
