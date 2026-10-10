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
	"time"
)

func TestHeader(t *testing.T) {
	a, b := net.Pipe()
	s := newSession("33600/31200", "555\n1000", "", time.Unix(1791666309, 0))
	s.User, s.Password = "alice", "secret"
	go func() {
		writeHeader(a, s, false)
		a.Write([]byte("data"))
	}()
	h, err := readHeader(b, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if h["MODEM_USER"] != "alice" || h["MODEM_DIALED"] != "555 1000" || h["MODEM_PASSWORD"] != "" {
		t.Errorf("header %v", h)
	}
	rest := make([]byte, 4)
	b.Read(rest)
	if string(rest) != "data" {
		t.Errorf("after the header: %q", rest)
	}
	env := strings.Join(headerEnv(h, false), "\n")
	if !strings.Contains(env, "DMODEM_RATE=33600/31200") || !strings.Contains(env, "DMODEM_CONNECTED=1791666309") ||
		strings.Contains(env, "UPSTREAM") {
		t.Errorf("env %s", env)
	}
	if env := strings.Join(headerEnv(h, true), "\n"); !strings.Contains(env, "MODEMMENU_UPSTREAM_USER=alice") {
		t.Errorf("trusted env %s", env)
	}
}

// modemmenu -serve on a unix socket: a session from app_dmodem's header,
// a hangup, and a chain from another modemmenu that logged the caller in
func TestServe(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if _, _, err := openPty(); err != nil {
		t.Skip("no pty:", err)
	}
	sock := filepath.Join(dir, "menu.sock")
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	// the served menu: trusts the user of a header
	conf := filepath.Join(dir, "server.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
banner = "Served line\r\n"
[file]
path = %s
[server]
listen = %s
trust_user = yes
socket_mode = 0600
[item info]
key = I
label = Info
command = echo "on $(tty | cut -c1-8) dialed=$MODEM_DIALED rate=$MODEM_RATE date=$MODEM_DATE auth=$MODEM_AUTH user=$MODEM_USER pw=$MODEM_PASSWORD up=$MODEMMENU_UPSTREAM_USER"
`, filepath.Join(dir, "server.log"), users, sock)), 0o600)

	srv := exec.Command(bin, "-config", conf, "-serve")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Process.Kill()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket: %v %v", fi, err)
	}

	// as app_dmodem with header = yes: the call's details, then a login
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("DMODEM_RATE=31200/26400\nDMODEM_CALLERID=5559999\nDMODEM_DIALED=5551000\n" +
		"DMODEM_CONNECTED=1791666309\nDMODEM_CHANNEL=PJSIP/trunk-1\n\n"))
	c := newCaller(t, conn)
	c.expect("Served line")
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection: ")
	c.send("info\r")
	c.expect("on /dev/pts dialed=5551000 rate=31200/26400 date=2026-10-10 auth=file user=alice pw=secret up=\n")
	c.expect("Goodbye")
	conn.Close()

	// a caller that hangs up at the login: the session ends
	conn, err = net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("\n"))
	c = newCaller(t, conn)
	c.expect("login: ")
	conn.Close()

	// another menu (on app_dmodem's pty) logs the caller in and hands
	// them over: no second login
	front := filepath.Join(dir, "front.conf")
	os.WriteFile(front, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
[file]
path = %s
[item more]
key = M
label = More services
mode = unix
path = %s
header = yes
`, users, sock)), 0o600)
	m, s := testPty(t)
	defer m.Close()
	cmd := exec.Command(bin, "-config", front)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.Env = append(os.Environ(), "DMODEM_RATE=26400/26400", "DMODEM_DIALED=5552000")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	defer cmd.Process.Kill()
	c = newCaller(t, m)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection: ")
	c.send("more\r")
	c.expect("Served line")
	c.expect("Welcome, alice.")
	c.expect("Selection: ")
	c.send("i\r")
	c.expect("dialed=5552000 rate=26400/26400 date=")
	c.expect("auth=upstream:file user=alice pw=secret up=\n")
	c.expect("Goodbye")
	cmd.Wait()

	// SIGTERM: the socket goes
	srv.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- srv.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the server is still running after SIGTERM")
	}
	if _, err := os.Stat(sock); err == nil {
		t.Error("the socket is still there")
	}
	log, _ := os.ReadFile(filepath.Join(dir, "server.log"))
	for _, w := range []string{"serving on", "login user=alice auth=file", "login user=alice auth=upstream:file",
		"callerid=\"5559999\" dialed=\"5551000\""} {
		if !bytes.Contains(log, []byte(w)) {
			t.Errorf("no %q in the log:\n%s", w, log)
		}
	}
	if n := bytes.Count(log, []byte("ended")); n != 3 {
		t.Errorf("%d sessions ended, want 3:\n%s", n, log)
	}
}
