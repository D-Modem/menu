// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Drop files: what a BBS door reads of its caller (who, the line, the
// time left), DOOR32.SYS and DOOR.SYS, written for an exec item with
// dropfile = before its program starts, removed after.  Doors want a node
// number of their own per caller and a number for each user: the nodes
// are lock files in state_dir/nodes, held while the door runs; the users'
// numbers are given once each in state_dir/usernumbers.

// dropfileSettings: [dropfile]
type dropfileSettings struct {
	bbsName   string
	sysop     string
	security  int
	emulation string // ansi, ascii
	comType   int    // DOOR32.SYS's comm type: 0 local, 1 serial, 2 telnet
	comHandle int    // DOOR32.SYS's comm handle
	comPort   string // DOOR.SYS's port, COM1:
	maxNodes  int
}

func loadDropfile(sec *section) (dropfileSettings, error) {
	var err error
	d := dropfileSettings{
		bbsName:   sec.get("bbs_name", "modemmenu"),
		sysop:     sec.get("sysop", "Sysop"),
		emulation: strings.ToLower(sec.get("emulation", "ansi")),
		comPort:   sec.get("com_port", "COM1:"),
	}
	if d.security, err = sec.int("security", 10); err != nil {
		return d, err
	}
	if d.comType, err = sec.int("comm_type", 2); err != nil {
		return d, err
	}
	if d.comHandle, err = sec.int("comm_handle", 0); err != nil {
		return d, err
	}
	if d.maxNodes, err = sec.int("max_nodes", 64); err != nil {
		return d, err
	}
	if d.emulation != "ansi" && d.emulation != "ascii" {
		return d, fmt.Errorf("[dropfile] emulation: %q (ansi or ascii)", d.emulation)
	}
	if d.comType < 0 || d.comType > 2 {
		return d, fmt.Errorf("[dropfile] comm_type: %d (0 local, 1 serial, 2 telnet)", d.comType)
	}
	if d.maxNodes < 1 {
		return d, fmt.Errorf("[dropfile] max_nodes: %d", d.maxNodes)
	}
	return d, nil
}

var dropfileNames = map[string]string{"door32.sys": "DOOR32.SYS", "door.sys": "DOOR.SYS"}

// dropfiles: what an item's dropfile = names, the files' names
func loadItemDropfiles(name string, sec *section) ([]string, error) {
	var out []string
	for _, f := range fields(strings.ToLower(sec.get("dropfile", ""))) {
		n, ok := dropfileNames[f]
		if !ok {
			return nil, fmt.Errorf("[item %s] dropfile: %q (door32.sys, door.sys)", name, f)
		}
		out = append(out, n)
	}
	return out, nil
}

// dropfiles: the files written for a door, and its node, until close
type dropfiles struct {
	node    int
	dir     string
	files   []string
	tempDir bool
	lock    *os.File
}

func (d *dropfiles) close() {
	for _, f := range d.files {
		os.Remove(f)
	}
	if d.tempDir {
		os.Remove(d.dir)
	}
	if d.lock != nil {
		d.lock.Close() // the node is free again
	}
}

// writeDropfiles for it's door: a node, the directory (dropfile_dir, or a
// new one), the files in it, owned by uid/gid if not -1
func writeDropfiles(cfg *settings, it *menuItem, s *session, deadline time.Time, uid, gid int) (*dropfiles, error) {
	dir := cfg.limits.stateDir
	node, lock, err := takeNode(dir, cfg.dropfile.maxNodes)
	if err != nil {
		return nil, err
	}
	d := &dropfiles{node: node, lock: lock}
	userNo, err := userNumber(dir, s.User)
	if err != nil {
		d.close()
		return nil, err
	}
	if it.dropfileDir != "" {
		v := s.vars(false)
		v["node"] = strconv.Itoa(node)
		d.dir = expand(it.dropfileDir, v, nil)
		if err := os.MkdirAll(d.dir, 0o755); err != nil {
			d.close()
			return nil, err
		}
	} else {
		if d.dir, err = os.MkdirTemp("", "modemmenu-node"+strconv.Itoa(node)+"-"); err != nil {
			d.close()
			return nil, err
		}
		d.tempDir = true
		if uid >= 0 {
			os.Chown(d.dir, uid, gid)
		}
	}
	minutes, seconds := 1440, 86400 // no limit: a day
	if !deadline.IsZero() {
		left := time.Until(deadline)
		if left < 0 {
			left = 0
		}
		minutes, seconds = int(left/time.Minute), int(left/time.Second)
	}
	security := cfg.dropfile.security
	if it.security > 0 {
		security = it.security
	}
	in := dropfileInfo{s: s, df: cfg.dropfile, node: node, userNo: userNo, security: security,
		minutes: minutes, seconds: seconds, now: time.Now()}
	for _, name := range it.dropfiles {
		var lines []string
		if name == "DOOR32.SYS" {
			lines = in.door32()
		} else {
			lines = in.doorSys()
		}
		path := filepath.Join(d.dir, name)
		// DOS's line ends: what doors read
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o600); err != nil {
			d.close()
			return nil, err
		}
		if uid >= 0 {
			os.Chown(path, uid, gid)
		}
		d.files = append(d.files, path)
	}
	return d, nil
}

type dropfileInfo struct {
	s                *session
	df               dropfileSettings
	node, userNo     int
	security         int
	minutes, seconds int
	now              time.Time
}

func (in dropfileInfo) baud() string {
	if in.s.Baud != "" {
		return in.s.Baud
	}
	return "0"
}

// door32: DOOR32.SYS, its 11 lines
func (in dropfileInfo) door32() []string {
	emu := "1"
	if in.df.emulation == "ascii" {
		emu = "0"
	}
	return []string{
		strconv.Itoa(in.df.comType),   // comm type: 0 local, 1 serial, 2 telnet
		strconv.Itoa(in.df.comHandle), // comm or socket handle
		in.baud(),                     // baud rate
		in.df.bbsName,                 // BBS software
		strconv.Itoa(in.userNo),       // user record number
		in.s.User,                     // real name
		in.s.User,                     // handle
		strconv.Itoa(in.security),     // security level
		strconv.Itoa(in.minutes),      // time left, minutes
		emu,                           // emulation: 0 ASCII, 1 ANSI
		strconv.Itoa(in.node),         // node
	}
}

// doorSys: DOOR.SYS (the GAP format), its 52 lines
func (in dropfileInfo) doorSys() []string {
	date := in.now.Format("01/02/06")
	graphics := "GR"
	if in.df.emulation == "ascii" {
		graphics = "NG"
	}
	return []string{
		in.df.comPort,             // 1 port, COM0: local
		in.baud(),                 // 2 baud rate
		"8",                       // 3 data bits
		strconv.Itoa(in.node),     // 4 node
		in.baud(),                 // 5 DTE rate
		"Y",                       // 6 screen display
		"N",                       // 7 printer
		"N",                       // 8 page bell
		"N",                       // 9 caller alarm
		in.s.User,                 // 10 full name
		"",                        // 11 calling from
		in.s.CallerID,             // 12 home phone
		in.s.CallerID,             // 13 work/data phone
		"",                        // 14 password: not given to doors
		strconv.Itoa(in.security), // 15 security level
		"1",                       // 16 times on
		date,                      // 17 last call
		strconv.Itoa(in.seconds),  // 18 seconds left
		strconv.Itoa(in.minutes),  // 19 minutes left
		graphics,                  // 20 graphics: GR ANSI, NG ASCII
		"24",                      // 21 screen length
		"Y",                       // 22 expert mode
		"",                        // 23 conferences
		"",                        // 24 conference
		"12/31/99",                // 25 expiration
		strconv.Itoa(in.userNo),   // 26 user record number
		"Z",                       // 27 protocol: ZMODEM
		"0",                       // 28 uploads
		"0",                       // 29 downloads
		"0",                       // 30 K downloaded today
		"0",                       // 31 K a day at most
		"01/01/80",                // 32 birth date
		"",                        // 33 main directory
		"",                        // 34 GEN directory
		in.df.sysop,               // 35 sysop
		in.s.User,                 // 36 alias
		"00:00",                   // 37 event time
		"Y",                       // 38 error-correcting connection
		"N",                       // 39 ANSI in NG mode
		"Y",                       // 40 record locking
		"7",                       // 41 default color
		"0",                       // 42 time credits, minutes
		date,                      // 43 last new files scan
		in.now.Format("15:04"),    // 44 time of this call
		in.now.Format("15:04"),    // 45 time of the last call
		"0",                       // 46 files a day at most
		"0",                       // 47 files downloaded today
		"0",                       // 48 K uploaded
		"0",                       // 49 K downloaded
		"",                        // 50 comment
		"0",                       // 51 doors opened
		"0",                       // 52 messages left
	}
}

// takeNode: the first free node of max (its lock file in dir/nodes, held
// until closed)
func takeNode(dir string, max int) (int, *os.File, error) {
	nodes := filepath.Join(dir, "nodes")
	if err := os.MkdirAll(nodes, 0o700); err != nil {
		return 0, nil, err
	}
	for n := 1; n <= max; n++ {
		f, err := os.OpenFile(filepath.Join(nodes, strconv.Itoa(n)), os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return 0, nil, err
		}
		if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil {
			return n, f, nil
		}
		f.Close()
	}
	return 0, nil, fmt.Errorf("all %d nodes are in use", max)
}

// userNumber: user's number, given the first time (1, 2, ...) and kept in
// dir/usernumbers
func userNumber(dir, user string) (int, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "usernumbers"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return 0, err
	}
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		num, name, ok := strings.Cut(sc.Text(), "\t")
		if !ok {
			continue
		}
		if name == user {
			return strconv.Atoi(num)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	n++
	_, err = fmt.Fprintf(f, "%d\t%s\n", n, user)
	return n, err
}
