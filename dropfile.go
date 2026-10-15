// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Drop files: what a BBS door reads of its caller (who, the line, the
// time left), written for an exec item with dropfile = before its
// program starts, removed after: DOOR32.SYS, DOOR.SYS and DORINFO1.DEF
// (or DORINFO#.DEF, # the node), in the layouts the Synchronet, Mystic
// and MBSE BBSs write.  Doors want a node number of their own per caller
// and a number for each user: the nodes are lock files in
// state_dir/nodes, held while the door runs; the users' numbers are
// given once each in state_dir/usernumbers.

// dropfileSettings: [dropfile]
type dropfileSettings struct {
	bbsName   string
	sysop     string
	security  int
	emulation string // ansi, ascii
	// how the door reaches the caller: DOOR32.SYS's comm type (0 local:
	// stdin and stdout, 1 serial, 2 telnet) and handle (-1 for local),
	// DOOR.SYS's and DORINFO's port (COM1:, COM0: local), DORINFO's
	// FOSSIL flag
	comType   int
	comHandle int
	comPort   string
	fossil    bool
	// the rate the files give: a fixed DTE rate (doors built on DMlib
	// and others refuse rates a FOSSIL does not have, such as 26400), or
	// "connect": the modems'
	rate           string
	fourDigitYears bool
	maxNodes       int
}

func loadDropfile(sec *section) (dropfileSettings, error) {
	var err error
	d := dropfileSettings{
		bbsName:   sec.get("bbs_name", "modemmenu"),
		sysop:     sec.get("sysop", "Sysop"),
		emulation: strings.ToLower(sec.get("emulation", "ansi")),
		comPort:   strings.ToUpper(sec.get("com_port", "COM1:")),
		rate:      strings.ToLower(sec.get("rate", "38400")),
	}
	if d.security, err = sec.int("security", 10); err != nil {
		return d, err
	}
	if d.comType, err = sec.int("comm_type", 0); err != nil {
		return d, err
	}
	defHandle := 0
	if d.comType == 0 {
		defHandle = -1
	}
	if d.comHandle, err = sec.int("comm_handle", defHandle); err != nil {
		return d, err
	}
	if d.fossil, err = sec.bool("fossil", true); err != nil {
		return d, err
	}
	if d.fourDigitYears, err = sec.bool("four_digit_years", false); err != nil {
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
	if !regexp.MustCompile(`^COM[0-9]:?$`).MatchString(d.comPort) {
		return d, fmt.Errorf("[dropfile] com_port: %q (COM0: to COM9:)", d.comPort)
	}
	if _, err := strconv.Atoi(d.rate); err != nil && d.rate != "connect" {
		return d, fmt.Errorf("[dropfile] rate: %q (a number, or connect)", d.rate)
	}
	if d.maxNodes < 1 {
		return d, fmt.Errorf("[dropfile] max_nodes: %d", d.maxNodes)
	}
	return d, nil
}

// the drop files an item's dropfile = names
var dropfileNames = map[string]string{
	"door32.sys": "DOOR32.SYS", "door.sys": "DOOR.SYS",
	"dorinfo1.def": "DORINFO1.DEF", "dorinfo#.def": "DORINFO#.DEF",
}

func loadItemDropfiles(name string, sec *section) ([]string, error) {
	var out []string
	for _, f := range fields(strings.ToLower(sec.get("dropfile", ""))) {
		n, ok := dropfileNames[f]
		if !ok {
			return nil, fmt.Errorf("[item %s] dropfile: %q (door32.sys, door.sys, dorinfo1.def or dorinfo#.def)", name, f)
		}
		out = append(out, n)
	}
	return out, nil
}

// checkDropfiles: DORINFO#.DEF names nodes 1 to 15 (one hex digit)
func checkDropfiles(s *settings) error {
	for _, it := range s.items {
		if contains(it.dropfiles, "DORINFO#.DEF") && s.dropfile.maxNodes > 15 {
			return fmt.Errorf("[item %s] dropfile dorinfo#.def: its name has the node as one hex digit, 1 to F: [dropfile] max_nodes must be 15 or less (it is %d)", it.name, s.dropfile.maxNodes)
		}
	}
	return nil
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
		minutes: minutes, seconds: seconds, now: time.Now(), doorSysLines: it.doorSysLines}
	for _, name := range it.dropfiles {
		var lines []string
		switch name {
		case "DOOR32.SYS":
			lines = in.door32()
		case "DOOR.SYS":
			lines = in.doorSys()
		default:
			lines = in.dorinfo()
			if name == "DORINFO#.DEF" {
				name = fmt.Sprintf("DORINFO%X.DEF", node)
			}
		}
		if it.dropfileLower {
			name = strings.ToLower(name)
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
	doorSysLines     int // 52, or 31 (then the last 21 blank)
}

// rate: the rate the files give
func (in dropfileInfo) rate() string {
	if in.df.rate != "connect" {
		return in.df.rate
	}
	if in.s.Baud != "" {
		return in.s.Baud
	}
	return "0"
}

func (in dropfileInfo) date(t time.Time) string {
	if in.df.fourDigitYears {
		return t.Format("01/02/2006")
	}
	return t.Format("01/02/06")
}

// split: a name's first word and the rest
func split(name string) (string, string) {
	first, rest, _ := strings.Cut(strings.TrimSpace(name), " ")
	return first, strings.TrimSpace(rest)
}

// door32: DOOR32.SYS (Mystic's Door32 revision 1), its 11 lines
func (in dropfileInfo) door32() []string {
	emu := "1"
	if in.df.emulation == "ascii" {
		emu = "0"
	}
	return []string{
		strconv.Itoa(in.df.comType),   // comm type: 0 local, 1 serial, 2 telnet
		strconv.Itoa(in.df.comHandle), // comm or socket handle (-1 local)
		in.rate(),                     // baud rate
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

// doorSys: DOOR.SYS (the GAP format), its 52 lines; with door_sys_lines =
// 31 the first 31, then the rest blank (as Synchronet writes it, for
// doors that do not take the longer one)
func (in dropfileInfo) doorSys() []string {
	date := in.date(in.now)
	graphics := "GR"
	if in.df.emulation == "ascii" {
		graphics = "NG"
	}
	port := strings.TrimSuffix(in.df.comPort, ":") + ":"
	lines := []string{
		port,                      // 1 port, COM0: local
		in.rate(),                 // 2 baud rate
		"8",                       // 3 data bits
		strconv.Itoa(in.node),     // 4 node
		in.rate(),                 // 5 DTE rate
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
		"0",                       // 24 conference
		in.date(time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)), // 25 expiration
		strconv.Itoa(in.userNo),                                // 26 user record number
		"Z",                                                    // 27 protocol: ZMODEM
		"0",                                                    // 28 uploads
		"0",                                                    // 29 downloads
		"0",                                                    // 30 K downloaded today
		"999999",                                               // 31 K a day at most
		in.date(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)), // 32 birth date
		"",                     // 33 main directory
		"",                     // 34 GEN directory
		in.df.sysop,            // 35 sysop
		in.s.User,              // 36 alias
		"00:00",                // 37 event time
		"Y",                    // 38 error-correcting connection
		"N",                    // 39 ANSI in NG mode
		"Y",                    // 40 record locking
		"7",                    // 41 default color
		"0",                    // 42 time credits, minutes
		date,                   // 43 last new files scan
		in.now.Format("15:04"), // 44 time of this call
		in.now.Format("15:04"), // 45 time of the last call
		"32768",                // 46 files a day at most
		"0",                    // 47 files downloaded today
		"0",                    // 48 K uploaded
		"0",                    // 49 K downloaded
		"",                     // 50 comment
		"0",                    // 51 doors opened
		"0",                    // 52 messages left
	}
	if in.doorSysLines == 31 {
		for i := 31; i < len(lines); i++ {
			lines[i] = ""
		}
	}
	return lines
}

// dorinfo: DORINFO1.DEF (RBBS-PC, QuickBBS, RemoteAccess), its 13 lines,
// in upper case as RBBS wrote it; the names split at their first space
func (in dropfileInfo) dorinfo() []string {
	sysFirst, sysLast := split(in.df.sysop)
	first, last := split(in.s.User)
	graphics := "1"
	if in.df.emulation == "ascii" {
		graphics = "0"
	}
	fossil := "0"
	if in.df.fossil {
		fossil = "-1"
	}
	lines := []string{
		in.df.bbsName,                          // 1 BBS name
		sysFirst,                               // 2 sysop's first name
		sysLast,                                // 3 sysop's last name
		strings.TrimSuffix(in.df.comPort, ":"), // 4 port: COM1, COM0 local
		in.rate() + " BAUD,N,8,1",              // 5 rate and line settings
		"0",                                    // 6 networked: no
		first,                                  // 7 caller's first name
		last,                                   // 8 caller's last name
		"",                                     // 9 caller's city
		graphics,                               // 10 graphics: 0 ASCII, 1 ANSI
		strconv.Itoa(in.security),              // 11 security level
		strconv.Itoa(in.minutes),               // 12 minutes left
		fossil,                                 // 13 FOSSIL: -1 yes, 0 no
	}
	for i := range lines {
		lines[i] = strings.ToUpper(lines[i])
	}
	return lines
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
