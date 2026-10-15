// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
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

func TestDropfileFormats(t *testing.T) {
	s := &session{User: "alice", Baud: "33600", CallerID: "5559999"}
	in := dropfileInfo{s: s, df: dropfileSettings{bbsName: "Test BBS", sysop: "Sysop", emulation: "ansi",
		comType: 2, comPort: "COM1:"}, node: 3, userNo: 7, security: 50, minutes: 42, seconds: 2530,
		now: time.Date(2026, 10, 15, 21, 5, 0, 0, time.UTC)}
	d32 := in.door32()
	if len(d32) != 11 || d32[0] != "2" || d32[2] != "33600" || d32[3] != "Test BBS" || d32[4] != "7" ||
		d32[5] != "alice" || d32[7] != "50" || d32[8] != "42" || d32[9] != "1" || d32[10] != "3" {
		t.Errorf("DOOR32.SYS %q", d32)
	}
	ds := in.doorSys()
	if len(ds) != 52 || ds[0] != "COM1:" || ds[3] != "3" || ds[9] != "alice" || ds[13] != "" ||
		ds[14] != "50" || ds[16] != "10/15/26" || ds[17] != "2530" || ds[18] != "42" || ds[19] != "GR" ||
		ds[25] != "7" || ds[34] != "Sysop" {
		t.Errorf("DOOR.SYS %q", ds)
	}
	in.df.emulation = "ascii"
	if in.door32()[9] != "0" || in.doorSys()[19] != "NG" {
		t.Error("ascii")
	}
}

func TestNodesAndUserNumbers(t *testing.T) {
	dir := t.TempDir()
	n1, l1, err := takeNode(dir, 2)
	if err != nil || n1 != 1 {
		t.Fatalf("first node %d %v", n1, err)
	}
	n2, l2, err := takeNode(dir, 2)
	if err != nil || n2 != 2 {
		t.Fatalf("second node %d %v", n2, err)
	}
	if _, _, err := takeNode(dir, 2); err == nil {
		t.Error("a third node of 2")
	}
	l1.Close()
	if n, l, err := takeNode(dir, 2); err != nil || n != 1 {
		t.Errorf("node 1 again: %d %v", n, err)
	} else {
		l.Close()
	}
	l2.Close()
	a, _ := userNumber(dir, "alice")
	b, _ := userNumber(dir, "bob")
	c, _ := userNumber(dir, "carol")
	if a == b || a < 1 || b < 1 || c != 3 {
		t.Errorf("numbers %d %d %d", a, b, c)
	}
	if a2, _ := userNumber(dir, "alice"); a2 != a {
		t.Errorf("alice again: %d, was %d", a2, a)
	}
}

// a door's drop files on a call, and the time left in its environment
func TestDropfileLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	out := filepath.Join(dir, "out")
	os.MkdirAll(out, 0o700)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
time_limit = 10m
state_dir = %s
[file]
path = %s
[dropfile]
bbs_name = Test BBS
[item door]
dropfile = door32.sys door.sys
security = 90
command = cp {dropfile_dir}/DOOR32.SYS {dropfile_dir}/DOOR.SYS %s/; echo DOOR node={node} "$MODEM_NODE left=$MODEM_TIME_LEFT/$MODEM_SECONDS_LEFT dir=$MODEM_DROPFILE_DIR"
after = menu
`, filepath.Join(dir, "state"), users, out)), 0o600)

	c, cmd := callAs(t, bin, conf, "DMODEM_RATE=26400/24000")
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection: ")
	c.send("door\r")
	got := c.expect("Selection: ")
	if !strings.Contains(got, "DOOR node=1 1 left=9/59") && !strings.Contains(got, "DOOR node=1 1 left=10/600") {
		t.Errorf("the door's line:\n%q", got)
	}
	c.send("g\r")
	waitExit(t, cmd)

	d32, _ := os.ReadFile(filepath.Join(out, "DOOR32.SYS"))
	lines := strings.Split(string(d32), "\r\n")
	if len(lines) != 12 || lines[2] != "26400" || lines[3] != "Test BBS" || lines[5] != "alice" ||
		lines[7] != "90" || (lines[8] != "9" && lines[8] != "10") || lines[10] != "1" {
		t.Errorf("DOOR32.SYS %q", d32)
	}
	ds, _ := os.ReadFile(filepath.Join(out, "DOOR.SYS"))
	if n := strings.Count(string(ds), "\r\n"); n != 52 {
		t.Errorf("DOOR.SYS: %d lines", n)
	}
	// the files and their directory are gone, the node free
	i := strings.Index(got, "dir=")
	tmp := strings.TrimSpace(strings.SplitN(got[i+4:], "\r\n", 2)[0])
	if _, err := os.Stat(tmp); err == nil {
		t.Errorf("%s is still there", tmp)
	}
	if n, l, err := takeNode(filepath.Join(dir, "state"), 1); err != nil || n != 1 {
		t.Errorf("node 1 after the call: %d %v", n, err)
	} else {
		l.Close()
	}
}

// a downstream modemmenu -serve keeps the upstream menu's time limit,
// from the header's MODEM_SECONDS_LEFT
func TestUpstreamTimeLimit(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	sock := filepath.Join(dir, "more.sock")
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	down := filepath.Join(dir, "down.conf")
	os.WriteFile(down, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
[file]
path = %s
[server]
listen = %s
trust_user = yes
socket_mode = 0600
[menu]
prompt = "\r\nDown ({time_left}): "
[item nap]
command = echo "DOWN left=$MODEM_SECONDS_LEFT"; exec sleep 60
`, users, sock)), 0o600)
	srv := exec.Command(bin, "-config", down, "-serve")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { srv.Process.Signal(syscall.SIGTERM); srv.Wait() }()
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	up := filepath.Join(dir, "up.conf")
	os.WriteFile(up, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
time_limit = 5s
time_warning = 0
[file]
path = %s
[item more]
mode = unix
path = %s
header = yes
`, users, sock)), 0o600)
	c, cmd := callAs(t, bin, up)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection: ")
	c.send("more\r")
	c.expect("Down (0): ")
	start := time.Now()
	c.send("nap\r")
	got := c.expect("Your time is up.")
	if !strings.Contains(got, "DOWN left=") || strings.Contains(got, "DOWN left=\r") {
		t.Errorf("the downstream item had no time left:\n%q", got)
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Errorf("hung up after %v", d)
	}
	waitExit(t, cmd)
}

func TestHeaderTimeLeft(t *testing.T) {
	a, b := net.Pipe()
	s := newSession("", "", "", time.Now())
	s.TimeLeft, s.SecondsLeft = "4", "275"
	go func() { writeHeader(a, s, false); a.Close() }()
	h, err := readHeader(b, time.Second)
	if err != nil || h["MODEM_TIME_LEFT"] != "4" || h["MODEM_SECONDS_LEFT"] != "275" {
		t.Errorf("header %v %v", h, err)
	}
	env := strings.Join(headerEnv(h, false), "\n")
	if !strings.Contains(env, "MODEMMENU_UPSTREAM_SECONDS_LEFT=275") {
		t.Errorf("env %s", env)
	}
	delete(h, "MODEM_SECONDS_LEFT")
	if env := strings.Join(headerEnv(h, false), "\n"); !strings.Contains(env, "MODEMMENU_UPSTREAM_SECONDS_LEFT=240") {
		t.Errorf("from minutes: %s", env)
	}
}
