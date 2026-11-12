// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/D-Modem/menu/internal/confdoc"
	"github.com/D-Modem/menu/internal/config"
)

// The setup's pick lists: for a key whose values are a few known words,
// or are things the file or the machine already has (the items, the
// groups, the users, the system's accounts), the values to pick from,
// each with a note of what it is.  A closed list takes nothing else; an
// open one is suggestions, and a value of one's own may still be typed; a
// multi one is checked off, some at once.

// choice is a value to pick, and its note.
type choice struct{ value, note string }

// choiceSet is what a key may be picked from.
type choiceSet struct {
	list  []choice
	open  bool // a value of one's own may be typed
	multi bool // several at once, joined by spaces
}

func (c *choiceSet) values() []string {
	out := make([]string, len(c.list))
	for i, x := range c.list {
		out[i] = x.value
	}
	return out
}

func (c *choiceSet) notes() []string {
	out := make([]string, len(c.list))
	for i, x := range c.list {
		out[i] = x.note
	}
	return out
}

// has: whether v is one of the list's values (case aside)
func (c *choiceSet) has(v string) bool {
	for _, x := range c.list {
		if strings.EqualFold(x.value, v) {
			return true
		}
	}
	return false
}

// closed is a list of value, note pairs that takes nothing else.
func closed(kv ...string) *choiceSet { return pairs(false, false, kv) }

// suggest is a list of value, note pairs, a value of one's own allowed.
func suggest(kv ...string) *choiceSet { return pairs(true, false, kv) }

func pairs(open, multi bool, kv []string) *choiceSet {
	c := &choiceSet{open: open, multi: multi}
	for i := 0; i+1 < len(kv); i += 2 {
		c.list = append(c.list, choice{kv[i], kv[i+1]})
	}
	return c
}

// add: a value not in the list yet
func (c *choiceSet) add(v, note string) {
	if v != "" && !c.has(v) {
		c.list = append(c.list, choice{v, note})
	}
}

// choicesFor: what [sec] key is picked from; nil: it is typed
func (u *setupUI) choicesFor(sec string, kd *confdoc.Kind, k *confdoc.Key) *choiceSet {
	kind := ""
	if kd != nil {
		kind = kd.Name
	}
	switch kind + "." + k.Name {
	case "general.log":
		return suggest("syslog", "the system log (auth facility)", "none", "no log",
			"/var/log/modemmenu.log", "a file of its own")
	case "general.auth":
		c := pairs(false, true, []string{
			"file", "the users file ([file])", "ldap", "an LDAP directory ([ldap])",
			"sql", "an SQL database ([sql])", "system", "this machine's accounts by PAM ([system])"})
		return c
	case "general.password_echo":
		return suggest("*", "a star for each character", "#", "a hash for each character", "none", "nothing at all")
	case "general.time_limit", "general.daily_limit", "guest.time_limit":
		return suggest("0", "no limit", "15m", "15 minutes", "30m", "half an hour", "1h", "an hour", "2h", "two hours")
	case "general.time_warning":
		return suggest("5m 1m", "at 5 minutes and at 1 minute left", "2m", "at 2 minutes left",
			"10m 5m 1m", "at 10, 5 and 1 minutes left", "0", "no warnings")
	case "general.state_dir":
		return suggest("/var/lib/modemmenu", "the usual place")

	case "terminal.petscii":
		return closed("auto", "watch the login for a Commodore's keys", "yes", "every caller is a Commodore", "no", "never")
	case "terminal.charset":
		return closed("auto", "by the terminal", "utf8", "UTF-8", "cp437", "the IBM PC's (SyncTERM, BBS terminals)",
			"dec", "a DEC VT's line drawing", "ascii", "plain + - | only")

	case "file.hash":
		return closed("auto", "by each line's prefix ($2b$, {SHA256}...)", "plain", "the rest of the line is the password")
	case "file.path", "signup.file":
		c := suggest("/etc/asterisk/modemmenu.users", "the usual place")
		if p, ok := u.val("file", "path"); ok {
			c.add(p, "[file] path")
		}
		return c

	case "ldap.password_change":
		return closed("exop", "Password Modify (OpenLDAP, 389 DS and most)", "ad", "Active Directory (ldaps or starttls only)")
	case "ldap.url":
		return suggest("ldaps://ldap.example.com", "TLS from the start", "ldap://ldap.example.com:389", "plain (use starttls)")

	case "sql.driver":
		return closed("mysql", "MySQL", "mariadb", "MariaDB", "postgres", "PostgreSQL", "sqlite", "an SQLite file")
	case "sql.hash":
		return closed("auto", "by the stored password's prefix", "bcrypt", "bcrypt hashes", "sha256", "SHA-256, hex",
			"sha512", "SHA-512, hex", "ssha256", "salted SHA-256", "ssha512", "salted SHA-512",
			"plain", "the password as it is", "none", "the query checks it: any row is a yes")
	case "sql.update_hash":
		return closed("bcrypt", "store a bcrypt hash", "none", "the password as typed (the query hashes it)")
	case "sql.args":
		return closed("user", "the query has one placeholder: the user", "user password", "user, then password",
			"password user", "password, then user")
	case "sql.update_args":
		return suggest("password user", "new password, then user", "user password", "user, then new password",
			"password user old_password", "new password, user, old password")

	case "system.groups":
		return u.groupChoices(true, false)
	case "system.min_uid":
		return suggest("1000", "most Linux systems' first user", "500", "older systems' first user", "0", "every account")
	case "system.pam_service":
		return suggest("modemmenu", "/etc/pam.d/modemmenu (modemmenu.pam.sample)", "login", "the console's", "sshd", "SSH's")

	case "server.listen":
		return u.listenChoices(sec, "/run/modemmenu/modemmenu.sock", "tcp:127.0.0.1:2323")
	case "authd.listen":
		return u.listenChoices(sec, "/run/modemmenu/auth.sock", "")
	case "server.socket_mode", "authd.socket_mode":
		return suggest("0660", "the owner and the group", "0600", "the owner only", "0666", "anyone on the machine")
	case "server.socket_group", "authd.socket_group":
		return u.systemGroupChoices()
	case "authd.allow_users":
		c := u.systemUserChoices()
		c.multi = true
		return c

	case "ppp_pool.dns":
		c := suggest()
		c.multi = true
		if v, ok := u.val("ppp_pool", "local"); ok {
			c.add(v, "this server's PPP address (its resolver)")
		}
		for _, ns := range resolvConf() {
			c.add(ns, "this machine's DNS server")
		}
		c.add("1.1.1.1", "Cloudflare")
		c.add("8.8.8.8", "Google")
		c.add("9.9.9.9", "Quad9")
		return c
	case "ppp_pool.local":
		c := suggest("100.120.224.1", "the sample's subnet")
		return c

	case "autoppp.item", "number.item":
		return u.itemChoices(false)
	case "number.user":
		return u.userChoices(false)

	case "aol.networks":
		return pairs(false, true, []string{"aolnet", "AOLnet", "sprintnet", "SprintNet", "tymnet", "Tymnet",
			"p3", "a CCL script that starts P3 at once"})
	case "aol.sprintnet_speed":
		return suggest("11:14", "9600 bps and up (high-speed settings)", "11:12", "2400 bps")
	case "aol.host":
		return suggest("localhost:5190", "the BBS's AOL front end on this machine")

	case "dropfile.emulation":
		return closed("auto", "by the terminal detected", "ansi", "always ANSI", "ascii", "always plain text")
	case "dropfile.comm_type":
		return closed("0", "local: the door on its own input and output", "1", "serial", "2", "telnet")
	case "dropfile.com_port":
		return suggest("COM0:", "local", "COM1:", "the first port", "COM2:", "the second port")
	case "dropfile.rate":
		return suggest("38400", "a FOSSIL's usual rate", "19200", "", "57600", "", "115200", "", "connect", "the modem's real rate")
	case "dropfile.security", "item.security":
		c := suggest("10", "a caller", "50", "a trusted caller", "100", "a co-sysop", "255", "the sysop")
		if kind == "item" {
			c.list = append([]choice{{"0", "[dropfile]'s"}}, c.list...)
		}
		return c

	case "menu.items":
		return u.itemChoices(true)
	case "menu.fullscreen":
		return closed("auto", "a window for terminals that can have it", "no", "lines of text for everyone")
	case "menu.hangup":
		return suggest("G goodbye bye logoff", "the sample's", "G", "only G", "Q quit bye", "Q for quit")

	case "item.mode":
		return closed("exec", "a program on the line (command)",
			"telnet", "a telnet server (host, port)",
			"rlogin", "an rlogin server (host, port)",
			"tcp", "a TCP server, raw (host, port)",
			"unix", "a unix socket, raw (path)",
			"passwd", "built in: change your password",
			"callers", "built in: the last callers",
			"setup", "built in: this setup (sysops only)")
	case "item.tty":
		return closed("raw", "the line as it is", "sane", "a normal terminal: echo, CR to LF")
	case "item.after":
		return closed("hangup", "hang up", "menu", "back to the menu")
	case "item.door_sys_lines":
		return closed("52", "the full DOOR.SYS", "31", "the old short one")
	case "item.dropfile":
		return pairs(false, true, []string{"door32.sys", "DOOR32.SYS (most newer doors)", "door.sys", "DOOR.SYS",
			"dorinfo1.def", "DORINFO1.DEF", "dorinfo#.def", "DORINFOn.DEF by the node"})
	case "item.terminal":
		return suggest("ANSI", "", "{term}", "what the caller's terminal said", "VT100", "", "VT220", "", "xterm", "")
	case "item.rlogin_terminal":
		return suggest("ansi/{baud}", "ANSI at the connect rate", "{term}/{baud}", "the caller's terminal type")
	case "item.run_as":
		c := u.systemUserChoices()
		c.list = append([]choice{{"{user}", "the caller's own account"}}, c.list...)
		return c
	case "item.users":
		return u.userChoices(true)
	case "item.groups":
		return u.groupChoices(true, true)
	case "item.min_baud":
		return suggest("2400", "", "9600", "", "14400", "", "28800", "", "33600", "V.34 at its best")
	case "item.host":
		return suggest("127.0.0.1", "this machine", "localhost", "this machine, by name", "::1", "this machine, IPv6")
	case "item.port":
		return suggest("23", "telnet", "513", "rlogin", "2323", "a BBS's telnet")
	case "item.path":
		c := suggest()
		for _, p := range sockets() {
			c.add(p, "a socket on this machine")
		}
		return c
	case "item.key":
		return u.keyChoices(sec)
	}
	// the sample's own: yes or no, or the values its help names
	if isBool(k.Value) {
		return closed("yes", "", "no", "")
	}
	if cs := choices(k); len(cs) > 1 {
		return &choiceSet{list: choiceNotes(k, cs)}
	}
	return nil
}

// choiceNotes: the notes the sample's help gives each value ("`passwd`:
// built in: ...; `callers`: ...")
func choiceNotes(k *confdoc.Key, cs []string) []choice {
	var out []choice
	for _, v := range cs {
		note := ""
		if i := strings.Index(k.Help, "`"+v+"`:"); i >= 0 {
			note = strings.TrimSpace(k.Help[i+len(v)+3:])
			if j := strings.Index(note, ";"); j >= 0 {
				note = note[:j]
			}
		}
		out = append(out, choice{v, note})
	}
	return out
}

// itemChoices: the [item NAME]s (multi: some, in the menu's order)
func (u *setupUI) itemChoices(multi bool) *choiceSet {
	c := &choiceSet{multi: multi}
	for _, s := range u.named("item") {
		name := strings.TrimPrefix(s, "item ")
		label, _ := u.val(s, "label")
		mode, _ := u.val(s, "mode")
		note := label
		if mode != "" && mode != "exec" {
			note += " (" + mode + ")"
		}
		c.list = append(c.list, choice{name, note})
	}
	return c
}

// takenKeys: what selects another item than [sec], or hangs up, and by
// what
func (u *setupUI) takenKeys(sec string) map[string]string {
	taken := map[string]string{}
	hangup, ok := u.val(menuSection, "hangup")
	if !ok {
		hangup = "G goodbye bye logoff"
	}
	for _, h := range config.Fields(strings.ToUpper(hangup)) {
		taken[h] = "the hangup"
	}
	for _, s := range u.named("item") {
		if s == sec {
			continue
		}
		name := strings.TrimPrefix(s, "item ")
		k, ok := u.val(s, "key")
		if !ok {
			k = name[:1]
		}
		taken[strings.ToUpper(k)] = "taken by [" + s + "]"
		taken[strings.ToUpper(name)] = "taken by [" + s + "]"
		v, _ := u.val(s, "aliases")
		for _, a := range config.Fields(strings.ToUpper(v)) {
			taken[a] = "taken by [" + s + "]"
		}
	}
	return taken
}

// keyChoices: keys for [item NAME]: the letters of its name and label,
// each free or what has it already
func (u *setupUI) keyChoices(sec string) *choiceSet {
	c := suggest()
	taken := u.takenKeys(sec)
	name := strings.TrimPrefix(sec, "item ")
	label, _ := u.val(sec, "label")
	for _, r := range strings.ToUpper(name + label) {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			continue
		}
		k := string(r)
		if t, ok := taken[k]; ok {
			c.add(k, t)
		} else {
			c.add(k, "free")
		}
	}
	return c
}

// listenChoices: the socket's usual path, a TCP address, and the others'
// listen marked taken
func (u *setupUI) listenChoices(sec, path, tcp string) *choiceSet {
	c := suggest(path, "the usual place")
	if tcp != "" {
		c.add(tcp, "TCP on this machine only")
	}
	for _, other := range []string{"server", "authd"} {
		if other == sec {
			continue
		}
		if v, ok := u.val(other, "listen"); ok {
			for i := range c.list {
				if c.list[i].value == v {
					c.list[i].note = "taken by [" + other + "] listen"
				}
			}
		}
	}
	return c
}

// userChoices: the users file's users (multi: some at once)
func (u *setupUI) userChoices(multi bool) *choiceSet {
	c := &choiceSet{open: true, multi: multi}
	path, ok := u.val("file", "path")
	if !ok {
		path = "/etc/asterisk/modemmenu.users"
	}
	for _, name := range usersIn(path) {
		c.add(name, "in the users file")
	}
	return c
}

// groupChoices: [groups]'s, and with system the system's groups
func (u *setupUI) groupChoices(system, multi bool) *choiceSet {
	c := &choiceSet{open: true, multi: multi}
	if !system || multi {
		for _, g := range u.doc.Keys("groups") {
			v, _ := u.val("groups", g)
			c.add(g, "[groups]: "+v)
		}
	}
	if system {
		for _, g := range systemGroups() {
			c.add(g, "a system group")
		}
	}
	return c
}

func (u *setupUI) systemGroupChoices() *choiceSet {
	c := &choiceSet{open: true}
	for _, g := range systemGroups() {
		c.add(g, "")
	}
	return c
}

func (u *setupUI) systemUserChoices() *choiceSet {
	c := &choiceSet{open: true}
	for _, n := range systemUsers() {
		c.add(n, "a system account")
	}
	return c
}

// usersIn: the names in a users file (user:hash lines)
func usersIn(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		if name, _, ok := strings.Cut(l, ":"); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}

// etcFiles: where the system's accounts are read from (tests change them)
var etcPasswd, etcGroup, etcResolv = "/etc/passwd", "/etc/group", "/etc/resolv.conf"

// systemUsers: /etc/passwd's accounts that log in (uid 1000 and up), and
// the services' few that a BBS uses
func systemUsers() []string {
	var out []string
	for _, f := range colonFile(etcPasswd) {
		if len(f) < 3 {
			continue
		}
		uid, _ := strconv.Atoi(f[2])
		if (uid >= 1000 && uid < 65534) || f[0] == "bbs" || f[0] == "asterisk" || f[0] == "nobody" {
			out = append(out, f[0])
		}
	}
	return out
}

// systemGroups: /etc/group's names
func systemGroups() []string {
	var out []string
	for _, f := range colonFile(etcGroup) {
		out = append(out, f[0])
	}
	sort.Strings(out)
	return out
}

func colonFile(path string) [][]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out [][]string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l == "" || l[0] == '#' {
			continue
		}
		out = append(out, strings.Split(l, ":"))
	}
	return out
}

// resolvConf: the machine's DNS servers
func resolvConf() []string {
	b, err := os.ReadFile(etcResolv)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[0] == "nameserver" && net.ParseIP(f[1]) != nil && !strings.HasPrefix(f[1], "127.") {
			out = append(out, f[1])
		}
	}
	return out
}

// sockets: the unix sockets of /run/modemmenu and /run/bbs
func sockets() []string {
	var out []string
	for _, dir := range []string{"/run/modemmenu", "/run/bbs"} {
		es, _ := os.ReadDir(dir)
		for _, e := range es {
			if e.Type()&os.ModeSocket != 0 {
				out = append(out, dir+"/"+e.Name())
			}
		}
	}
	return out
}

// val: a key's value in the file as it is now, and whether it is set
func (u *setupUI) val(sec, key string) (string, bool) { return u.doc.Get(sec, key) }
