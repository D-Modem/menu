// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPPPFrame: what starts a PPP frame, and what does not
func TestPPPFrame(t *testing.T) {
	for in, want := range map[string]bool{
		"\x7e\xff\x7d\x23\xc0\x21": true, // 0xFF 0x03 (escaped) LCP
		"\x7e\xff\x03\xc0\x21":     true,
		"\x7e\x7d\xdf\x7d\x23":     true, // the address escaped too
		"\x7e\xc0\x21\x01":         true, // address and control compressed
		"~alice":                   false,
		"\x7e\xff":                 false,
		"alice":                    false,
	} {
		if got := pppFrame([]byte(in)); got != want {
			t.Errorf("% x: %v, want %v", in, got, want)
		}
	}
}

// buildPlugin: modemmenu.so and the host that loads it as pppd would, or
// a skip without a C compiler and pppd's headers
func buildPlugin(t *testing.T, dir string) (plugin, host string) {
	t.Helper()
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no C compiler")
	}
	if _, err := os.Stat("/usr/include/pppd/pppd.h"); err != nil {
		t.Skip("no pppd headers (ppp-dev)")
	}
	plugin, host = filepath.Join(dir, "modemmenu.so"), filepath.Join(dir, "host")
	for _, args := range [][]string{
		{"-Wall", "-Werror", "-Wno-unused-parameter", "-fPIC", "-shared", "-o", plugin, "pppd/modemmenu.c"},
		{"-Wall", "-Werror", "-rdynamic", "-o", host, "pppd/test/host.c", "-ldl"},
	} {
		if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
			t.Fatalf("cc %v: %v\n%s", args, err, out)
		}
	}
	return plugin, host
}

// the plugin's PAP logins against a call's auth socket
func TestPPPAuth(t *testing.T) {
	dir := t.TempDir()
	plugin, host := buildPlugin(t, dir)
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\nbob:{PLAIN}secret\n"), 0o600)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
fail_delay = 0
attempts = 2
time_limit = 1h
[file]
path = %s
[autoppp]
enabled = yes
command = pppd
item = ppp
[item ppp]
users = alice
command = true
`, users)), 0o600)
	s, err := loadSettings(conf)
	if err != nil {
		t.Fatal(err)
	}
	pap := func(a *pppAuth, user, pw string) string {
		out, err := exec.Command(host, plugin, user, pw, a.path()).CombinedOutput()
		if err != nil {
			t.Fatalf("host: %v\n%s", err, out)
		}
		return string(out)
	}

	a, err := startPPPAuth(s, &session{})
	if err != nil {
		t.Fatal(err)
	}
	if out := pap(a, "alice", "wrong"); !strings.Contains(out, "auth=0 maxconnect=0 address=-1") {
		t.Errorf("a wrong password:\n%s", out)
	}
	out := pap(a, "alice", "secret")
	if !strings.Contains(out, "auth=1 maxconnect=35") || !strings.Contains(out, "address=1") {
		t.Errorf("the right one:\n%s", out)
	}
	if a.sess.User != "alice" || a.sess.Auth != "file" || a.lim == nil {
		t.Errorf("session %+v", a.sess)
	}
	if out := pap(a, "alice", "secret"); !strings.Contains(out, "auth=0") {
		t.Errorf("a second login in a call:\n%s", out)
	}
	a.close()

	// not in the item's users; then too many attempts
	a, _ = startPPPAuth(s, &session{})
	if out := pap(a, "bob", "secret"); !strings.Contains(out, "auth=0") || a.sess.User != "" {
		t.Errorf("bob, not allowed PPP:\n%s", out)
	}
	pap(a, "alice", "wrong") // the second of attempts = 2
	if out := pap(a, "alice", "secret"); !strings.Contains(out, "auth=0") {
		t.Errorf("after attempts tries:\n%s", out)
	}
	a.close()
	if _, err := os.Stat(a.dir); err == nil {
		t.Error("the socket's directory is still there")
	}
}

// a dialer's PPP frame at the login prompt: the [autoppp] command (the
// host, standing in for pppd) logs in by the plugin on the call's socket
func TestAutoPPPLine(t *testing.T) {
	dir := t.TempDir()
	plugin, host := buildPlugin(t, dir)
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	log := filepath.Join(dir, "log")
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
time_limit = 10m
[file]
path = %s
[autoppp]
enabled = yes
command = %s %s alice secret; echo "pppd saw dialed=$MODEM_DIALED socket=${MODEMMENU_AUTH_SOCKET:+yes}"
[item info]
command = true
`, log, users, host, plugin)), 0o600)

	c, cmd := callAs(t, bin, conf, "DMODEM_DIALED=5551000", "DMODEM_RATE=33600/33600")
	c.expect("login: ")
	// LCP Configure-Request, as a dialer sends it
	c.send("\x7e\xff\x7d\x23\xc0\x21\x7d\x21\x7d\x21\x7d\x20\x7d\x34")
	got := c.expect("pppd saw")
	if !strings.Contains(got, "auth=1 maxconnect=59") || !strings.Contains(got, "address=1") {
		t.Errorf("pppd's login:\n%q", got)
	}
	c.expect("dialed=5551000 socket=yes")
	waitExit(t, cmd)

	// a name that starts with ~ is a name
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("~x\r")
	c.expect("Password: ")
	c.send("y\r")
	c.expect("Login incorrect")
	cmd.Process.Kill()

	b, _ := os.ReadFile(log)
	for _, w := range []string{"PPP frame at the login prompt", "login user=alice auth=file via PPP", "logout user=alice: PPP ended"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("no %q in the log:\n%s", w, b)
		}
	}
}
