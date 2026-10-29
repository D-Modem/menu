// Copyright (C) 2026 D-Modem Authors
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

// TestDropfileFormats: DOOR32.SYS, DOOR.SYS (52 and 31 lines) and
// DORINFO1.DEF, line by line
func TestDropfileFormats(t *testing.T) {
	sec := &section{name: "dropfile", vals: map[string]string{"bbs_name": "Test BBS", "sysop": "Ada Lovelace"}}
	df, err := loadDropfile(sec)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{User: "alice", Baud: "26400", CallerID: "5559999"}
	in := dropfileInfo{s: s, df: df, node: 3, userNo: 7, security: 50, minutes: 42, seconds: 2530,
		now: time.Date(2026, 10, 15, 21, 5, 0, 0, time.UTC), doorSysLines: 52}

	// DOOR32.SYS: a stdio door is local, handle -1 (as Synchronet); the
	// DTE rate, 38400, not the modems' 26400
	d32 := in.door32()
	want32 := []string{"0", "-1", "38400", "Test BBS", "7", "alice", "alice", "50", "42", "1", "3"}
	if strings.Join(d32, "|") != strings.Join(want32, "|") {
		t.Errorf("DOOR32.SYS\n%q\nwant\n%q", d32, want32)
	}

	ds := in.doorSys()
	if len(ds) != 52 || ds[0] != "COM1:" || ds[1] != "38400" || ds[3] != "3" || ds[4] != "38400" ||
		ds[9] != "alice" || ds[11] != "5559999" || ds[13] != "" || ds[14] != "50" || ds[16] != "10/15/26" ||
		ds[17] != "2530" || ds[18] != "42" || ds[19] != "GR" || ds[24] != "12/31/99" || ds[25] != "7" ||
		ds[30] != "999999" || ds[34] != "Ada Lovelace" || ds[45] != "32768" || ds[51] != "0" {
		t.Errorf("DOOR.SYS %q", ds)
	}
	in.doorSysLines = 31
	if ds := in.doorSys(); len(ds) != 52 || ds[30] != "999999" || ds[31] != "" || ds[34] != "" || ds[51] != "" {
		t.Errorf("DOOR.SYS, 31 lines: %q", ds)
	}
	in.doorSysLines = 52

	// DORINFO1.DEF: 13 lines, upper case, names split at the first space
	di := in.dorinfo()
	wantDI := []string{"TEST BBS", "ADA", "LOVELACE", "COM1", "38400 BAUD,N,8,1", "0", "ALICE", "", "", "1", "50", "42", "-1"}
	if strings.Join(di, "|") != strings.Join(wantDI, "|") {
		t.Errorf("DORINFO1.DEF\n%q\nwant\n%q", di, wantDI)
	}
	in.s = &session{User: "bob smith jr"}
	if di := in.dorinfo(); di[6] != "BOB" || di[7] != "SMITH JR" {
		t.Errorf("a name with spaces: %q %q", di[6], di[7])
	}
	in.s = s

	// the modems' rate, ASCII, four-digit years, no FOSSIL, local port
	sec.vals["rate"], sec.vals["emulation"], sec.vals["four_digit_years"] = "connect", "ascii", "yes"
	sec.vals["fossil"], sec.vals["com_port"], sec.vals["comm_type"] = "no", "com0:", "2"
	if in.df, err = loadDropfile(sec); err != nil {
		t.Fatal(err)
	}
	d32, ds, di = in.door32(), in.doorSys(), in.dorinfo()
	if d32[0] != "2" || d32[1] != "0" || d32[2] != "26400" || d32[9] != "0" {
		t.Errorf("DOOR32.SYS %q", d32)
	}
	if ds[0] != "COM0:" || ds[1] != "26400" || ds[16] != "10/15/2026" || ds[19] != "NG" {
		t.Errorf("DOOR.SYS %q", ds)
	}
	if di[3] != "COM0" || di[4] != "26400 BAUD,N,8,1" || di[9] != "0" || di[12] != "0" {
		t.Errorf("DORINFO1.DEF %q", di)
	}

	for k, v := range map[string]string{"comm_type": "3", "com_port": "LPT1", "rate": "fast", "emulation": "rip", "max_nodes": "0"} {
		bad := &section{name: "dropfile", vals: map[string]string{k: v}}
		if _, err := loadDropfile(bad); err == nil {
			t.Errorf("%s = %s: no error", k, v)
		}
	}
}

// TestNodesAndUserNumbers: nodes are taken and let go under locks, and
// each user keeps one number
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
max_nodes = 15
[item door]
dropfile = door32.sys door.sys dorinfo#.def
security = 90
command = cp {dropfile_dir}/* %s/; echo DOOR node={node} "$MODEM_NODE left=$MODEM_TIME_LEFT/$MODEM_SECONDS_LEFT first=$(basename $MODEM_DROPFILE) dir=$MODEM_DROPFILE_DIR"
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
	if len(lines) != 12 || lines[0] != "0" || lines[1] != "-1" || lines[2] != "38400" || lines[3] != "Test BBS" ||
		lines[5] != "alice" || lines[7] != "90" || (lines[8] != "9" && lines[8] != "10") || lines[10] != "1" {
		t.Errorf("DOOR32.SYS %q", d32)
	}
	ds, _ := os.ReadFile(filepath.Join(out, "DOOR.SYS"))
	if n := strings.Count(string(ds), "\r\n"); n != 52 {
		t.Errorf("DOOR.SYS: %d lines", n)
	}
	// DORINFO#.DEF: the node as a hex digit
	di, err := os.ReadFile(filepath.Join(out, "DORINFO1.DEF"))
	if n := strings.Count(string(di), "\r\n"); err != nil || n != 13 || !strings.HasPrefix(string(di), "TEST BBS\r\n") {
		t.Errorf("DORINFO1.DEF: %d lines, %v:\n%q", n, err, di)
	}
	if !strings.Contains(got, "first=DOOR32.SYS") {
		t.Errorf("MODEM_DROPFILE: %q", got)
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

// TestHeaderTimeLeft: the time left goes through the session header and
// becomes the downstream menu's limit
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

// TestDropfileSettings: [dropfile] and dropfile = settings that load,
// and those refused
func TestDropfileSettings(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	load := func(extra string) error {
		p := filepath.Join(dir, "c.conf")
		os.WriteFile(p, []byte("[general]\nauth = file\n[file]\npath = "+users+"\n"+extra), 0o600)
		_, err := loadSettings(p)
		return err
	}
	if err := load("[item d]\ndropfile = dorinfo#.def\ncommand = true\n"); err == nil {
		t.Error("dorinfo#.def with 64 nodes: no error")
	}
	if err := load("[dropfile]\nmax_nodes = 15\n[item d]\ndropfile = dorinfo#.def door.sys\ndoor_sys_lines = 31\ndropfile_lowercase = yes\ncommand = true\n"); err != nil {
		t.Errorf("dorinfo#.def with 15 nodes: %v", err)
	}
	for _, bad := range []string{"dropfile = chain.txt", "dropfile = door.sys\ndoor_sys_lines = 40", "dropfile = door.sys\nmode = tcp"} {
		if err := load("[item d]\n" + bad + "\ncommand = true\n"); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// the files' names in lower case, for doors on case-sensitive systems
func TestDropfileLowercase(t *testing.T) {
	dir := t.TempDir()
	cfg := &settings{}
	cfg.limits.stateDir = dir
	cfg.dropfile, _ = loadDropfile(&section{name: "dropfile", vals: map[string]string{}})
	it := &menuItem{name: "d", dropfiles: []string{"DOOR32.SYS", "DORINFO1.DEF"}, dropfileLower: true, doorSysLines: 52}
	df, err := writeDropfiles(cfg, it, &session{User: "alice"}, time.Time{}, -1, -1)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"door32.sys", "dorinfo1.def"} {
		if _, err := os.Stat(filepath.Join(df.dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	// no limit: a day
	b, _ := os.ReadFile(filepath.Join(df.dir, "door32.sys"))
	if strings.Split(string(b), "\r\n")[8] != "1440" {
		t.Errorf("no limit: %q", b)
	}
	df.close()
	if _, err := os.Stat(df.dir); err == nil {
		t.Error("the directory is still there")
	}
}
