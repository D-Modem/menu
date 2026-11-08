// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/auth"
	"github.com/D-Modem/menu/internal/config"
	"github.com/D-Modem/menu/internal/term"
)

type general struct {
	log            string
	banner         string
	loginPrompt    string
	passwordPrompt string
	welcome        string
	connectMessage string // after the banner, when the rate is known
	motd           string
	motdFile       string
	goodbye        string
	attempts       int
	failDelay      time.Duration
	loginTimeout   time.Duration
	idleTimeout    time.Duration
	userPattern    *regexp.Regexp
	maxInput       int
	mask           string
	auth           []string
}

type menuItem struct {
	name          string
	key           string   // shown in the menu
	choices       []string // what selects it, upper case: key, name, aliases
	label         string
	mode          string // exec, tcp, rlogin, telnet
	command       string
	host          string
	port          int
	path          string // a unix socket instead of host and port
	header        bool   // the session header first
	handoff       bool   // mode unix: the line itself to the server
	send          string
	rloginLocal   string
	rloginUser    string
	rloginTerm    string
	terminal      string
	runAs         string
	tty           string // raw, sane
	password      bool
	sessionFile   bool
	after         string // hangup, menu
	timeout       time.Duration
	access        itemAccess
	minPassword   int      // mode = passwd
	timeWarning   bool     // the time_warning is written on the line
	dropfiles     []string // DOOR32.SYS, DOOR.SYS
	dropfileDir   string   // where ("": a new directory each time)
	security      int      // the drop files' security level (0: [dropfile]'s)
	doorSysLines  int      // DOOR.SYS: 52, or 31 (the rest blank)
	dropfileLower bool     // the files' names in lower case
	count         int      // mode = callers: how many
}

// serverSettings: [server], for modemmenu -serve
type serverSettings struct {
	listen        string // a unix socket's path, or tcp:host:port
	socketMode    os.FileMode
	socketGroup   string
	header        bool
	headerTimeout time.Duration
	trustUser     bool
	maxSessions   int
}

type settings struct {
	general
	server     *serverSettings
	groups     map[string][]string // [groups]: name = members
	guest      guestSettings
	signup     signupSettings
	callerID   callerIDSettings
	limits     limitSettings
	dropfile   dropfileSettings
	autoppp    autopppSettings
	pppPool    pppPoolSettings
	terminal   terminalSettings
	authd      *authdSettings // nil: no auth socket
	menuTitle  string
	menuPrompt string
	hangup     []string // what hangs up, upper case; the first one shown
	hotkeys    bool     // one key selects, without a line end
	items      []*menuItem
	backends   []auth.Authenticator
}

// loadSettings: the configuration file, every section loaded and
// checked
func loadSettings(path string) (*settings, error) {
	c, err := config.Parse(path)
	if err != nil {
		return nil, err
	}
	s := &settings{}
	g := c.Section("general")
	s.log = g.Get("log", "syslog")
	s.banner = config.Unescape(g.Get("banner", `\r\nD-Modem\r\n`))
	if f := g.Get("banner_file", ""); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("[general] banner_file: %v", err)
		}
		s.banner = term.CRLF(string(b))
	}
	s.loginPrompt = config.Unescape(g.Get("login_prompt", `\r\nlogin: `))
	s.passwordPrompt = config.Unescape(g.Get("password_prompt", "Password: "))
	s.welcome = config.Unescape(g.Get("welcome", `\r\nWelcome, {user}.\r\n`))
	s.connectMessage = config.Unescape(g.Get("connect_message", `\r\nConnected at {speed}.\r\n`))
	s.motd = config.Unescape(g.Get("motd", ""))
	s.motdFile = g.Get("motd_file", "")
	s.goodbye = config.Unescape(g.Get("goodbye", `\r\nGoodbye.\r\n`))
	s.mask = g.Get("password_echo", "*")
	if s.mask == "none" {
		s.mask = ""
	}
	if s.attempts, err = g.Int("attempts", 3); err != nil {
		return nil, err
	}
	if s.maxInput, err = g.Int("max_input", 64); err != nil {
		return nil, err
	}
	if s.failDelay, err = g.Duration("fail_delay", 3*time.Second); err != nil {
		return nil, err
	}
	if s.loginTimeout, err = g.Duration("login_timeout", 90*time.Second); err != nil {
		return nil, err
	}
	if s.idleTimeout, err = g.Duration("idle_timeout", 5*time.Minute); err != nil {
		return nil, err
	}
	pat := g.Get("user_pattern", `^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)
	if s.userPattern, err = regexp.Compile(pat); err != nil {
		return nil, fmt.Errorf("[general] user_pattern: %v", err)
	}
	s.auth = config.Fields(strings.ToLower(g.Get("auth", "file")))
	if len(s.auth) == 0 {
		return nil, fmt.Errorf("[general] auth: no authentication backend")
	}
	for _, name := range s.auth {
		var a auth.Authenticator
		switch name {
		case "file":
			a, err = auth.NewFile(c.Section("file"))
		case "ldap":
			a, err = auth.NewLDAP(c.Section("ldap"))
		case "sql":
			a, err = auth.NewSQL(c.Section("sql"))
		case "system", "pam":
			a, err = auth.NewSystem(c.Section("system"))
		default:
			err = fmt.Errorf("[general] auth: unknown backend %q (file, ldap, sql, system)", name)
		}
		if err != nil {
			return nil, err
		}
		s.backends = append(s.backends, a)
	}

	if s.server, err = loadServer(c.Section("server")); err != nil {
		return nil, err
	}
	if err := loadAccess(c, s); err != nil {
		return nil, err
	}
	if s.dropfile, err = loadDropfile(c.Section("dropfile")); err != nil {
		return nil, err
	}
	if s.autoppp, err = loadAutoPPP(c.Section("autoppp")); err != nil {
		return nil, err
	}
	if s.pppPool, err = loadPPPPool(c.Section("ppp_pool")); err != nil {
		return nil, err
	}
	if s.terminal, err = loadTerminal(c.Section("terminal")); err != nil {
		return nil, err
	}
	if s.authd, err = loadAuthd(c.Section("authd")); err != nil {
		return nil, err
	}

	m := c.Section("menu")
	s.menuTitle = config.Unescape(m.Get("title", `\r\nMain menu\r\n`))
	s.menuPrompt = config.Unescape(m.Get("prompt", `\r\nSelection: `))
	s.hangup = config.Fields(strings.ToUpper(m.Get("hangup", "G goodbye bye logoff")))
	if len(s.hangup) == 0 {
		return nil, fmt.Errorf("[menu] hangup: nothing to hang up with")
	}
	var names []string
	if m.Has("items") {
		names = config.Fields(m.Get("items", ""))
	} else {
		for _, sec := range c.Sections {
			if strings.HasPrefix(sec.Name, "item ") {
				names = append(names, strings.TrimPrefix(sec.Name, "item "))
			}
		}
	}
	if s.hotkeys, err = m.Bool("hotkeys", false); err != nil {
		return nil, err
	}
	keys := map[string]string{}
	for _, h := range s.hangup {
		keys[h] = "[menu] hangup"
	}
	for _, name := range names {
		sec, ok := c.ByName["item "+name]
		if !ok {
			return nil, fmt.Errorf("[menu] items: no [item %s]", name)
		}
		it, err := loadItem(name, sec)
		if err != nil {
			return nil, err
		}
		for _, c := range it.choices {
			if prev, dup := keys[c]; dup && prev != "[item "+name+"]" {
				return nil, fmt.Errorf("[item %s] %s: also selects %s", name, c, prev)
			}
			keys[c] = "[item " + name + "]"
		}
		s.items = append(s.items, it)
	}
	if len(s.items) == 0 {
		return nil, fmt.Errorf("%s: no [item ...] in the menu", path)
	}
	if err := checkAccess(s); err != nil {
		return nil, err
	}
	if err := checkDropfiles(s); err != nil {
		return nil, err
	}
	if err := checkPool(s); err != nil {
		return nil, err
	}
	if s.autoppp.item != "" && s.selects(strings.ToUpper(s.autoppp.item)) == nil {
		return nil, fmt.Errorf("[autoppp] item: no item %q", s.autoppp.item)
	}
	if s.hotkeys {
		for _, it := range s.items {
			if len(it.key) != 1 {
				return nil, fmt.Errorf("[item %s] key %s: hotkeys = yes takes one character", it.name, it.key)
			}
		}
		if len(s.hangup[0]) != 1 {
			return nil, fmt.Errorf("[menu] hangup: with hotkeys = yes, the first is one character")
		}
	}
	return s, nil
}

// loadItem: an [item ...] section, checked
func loadItem(name string, sec *config.Section) (*menuItem, error) {
	var err error
	it := &menuItem{name: name}
	it.key = strings.ToUpper(sec.Get("key", name[:1]))
	if it.key == "" || strings.ContainsAny(it.key, " \t,") {
		return nil, fmt.Errorf("[item %s] key: %q (one word)", name, it.key)
	}
	// the key, the item's name and its aliases all select it
	it.choices = []string{it.key}
	for _, c := range append([]string{name}, config.Fields(sec.Get("aliases", ""))...) {
		c = strings.ToUpper(c)
		if c != it.key && !contains(it.choices, c) {
			it.choices = append(it.choices, c)
		}
	}
	it.label = sec.Get("label", name)
	it.mode = strings.ToLower(sec.Get("mode", "exec"))
	it.command = sec.Get("command", "")
	it.host = sec.Get("host", "127.0.0.1")
	it.send = sec.Get("send", "")
	it.path = sec.Get("path", "")
	if it.header, err = sec.Bool("header", false); err != nil {
		return nil, err
	}
	if it.handoff, err = sec.Bool("handoff", false); err != nil {
		return nil, err
	}
	if it.handoff && (strings.ToLower(sec.Get("mode", "")) != "unix" || !it.header) {
		return nil, fmt.Errorf("[item %s] handoff = yes needs mode = unix and header = yes", name)
	}
	it.rloginLocal = sec.Get("rlogin_client_user", "{password}")
	it.rloginUser = sec.Get("rlogin_server_user", "{user}")
	it.rloginTerm = sec.Get("rlogin_terminal", "ansi/{baud}")
	it.terminal = sec.Get("terminal", "ANSI")
	it.runAs = sec.Get("run_as", "")
	it.tty = strings.ToLower(sec.Get("tty", "raw"))
	defAfter := "hangup"
	if m := strings.ToLower(sec.Get("mode", "")); m == "passwd" || m == "callers" {
		// a password changed is no reason to hang up
		defAfter = "menu"
	}
	it.after = strings.ToLower(sec.Get("after", defAfter))
	if it.minPassword, err = sec.Int("min_password", 8); err != nil {
		return nil, err
	}
	if it.timeWarning, err = sec.Bool("time_warning", true); err != nil {
		return nil, err
	}
	if it.count, err = sec.Int("count", 10); err != nil {
		return nil, err
	}
	if it.dropfiles, err = loadItemDropfiles(name, sec); err != nil {
		return nil, err
	}
	it.dropfileDir = sec.Get("dropfile_dir", "")
	if it.security, err = sec.Int("security", 0); err != nil {
		return nil, err
	}
	if it.doorSysLines, err = sec.Int("door_sys_lines", 52); err != nil {
		return nil, err
	}
	if it.doorSysLines != 52 && it.doorSysLines != 31 {
		return nil, fmt.Errorf("[item %s] door_sys_lines: %d (52 or 31)", name, it.doorSysLines)
	}
	if it.dropfileLower, err = sec.Bool("dropfile_lowercase", false); err != nil {
		return nil, err
	}
	if len(it.dropfiles) > 0 && strings.ToLower(sec.Get("mode", "exec")) != "exec" {
		return nil, fmt.Errorf("[item %s] dropfile is for mode = exec", name)
	}
	if it.password, err = sec.Bool("pass_password", true); err != nil {
		return nil, err
	}
	if it.sessionFile, err = sec.Bool("session_file", false); err != nil {
		return nil, err
	}
	if it.timeout, err = sec.Duration("connect_timeout", 10*time.Second); err != nil {
		return nil, err
	}
	if it.access, err = loadItemAccess(name, sec); err != nil {
		return nil, err
	}
	if it.mode == "unix" {
		// raw bytes over a unix socket: tcp with a path
		it.mode = "tcp"
		if it.path == "" {
			return nil, fmt.Errorf("[item %s] mode = unix needs a path", name)
		}
	}
	defPort := map[string]int{"tcp": 23, "telnet": 23, "rlogin": 513}[it.mode]
	if it.port, err = sec.Int("port", defPort); err != nil {
		return nil, err
	}
	switch it.mode {
	case "passwd", "callers":
		// built in: no command, no server
	case "exec":
		if it.command == "" {
			return nil, fmt.Errorf("[item %s] mode = exec needs a command", name)
		}
	case "tcp", "telnet", "rlogin":
		if it.port <= 0 || it.port > 65535 {
			return nil, fmt.Errorf("[item %s] port: %d", name, it.port)
		}
		if it.runAs != "" {
			return nil, fmt.Errorf("[item %s] run_as is for mode = exec", name)
		}
		if it.path != "" && !strings.HasPrefix(it.path, "/") {
			return nil, fmt.Errorf("[item %s] path: %q (an absolute path)", name, it.path)
		}
	default:
		return nil, fmt.Errorf("[item %s] mode: %q (exec, tcp, unix, telnet, rlogin, passwd or callers)", name, it.mode)
	}
	if it.tty != "raw" && it.tty != "sane" {
		return nil, fmt.Errorf("[item %s] tty: %q (raw or sane)", name, it.tty)
	}
	if it.after != "hangup" && it.after != "menu" {
		return nil, fmt.Errorf("[item %s] after: %q (hangup or menu)", name, it.after)
	}
	return it, nil
}

// contains: whether s is in list
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// loadServer: [server], for modemmenu -serve
func loadServer(sec *config.Section) (*serverSettings, error) {
	var err error
	srv := &serverSettings{
		listen:      sec.Get("listen", "/run/modemmenu/modemmenu.sock"),
		socketGroup: sec.Get("socket_group", ""),
	}
	mode, err := strconv.ParseUint(sec.Get("socket_mode", "0660"), 8, 32)
	if err != nil || mode > 0o777 {
		return nil, fmt.Errorf("[server] socket_mode: %q (octal, e.g. 0660)", sec.Get("socket_mode", ""))
	}
	srv.socketMode = os.FileMode(mode)
	if srv.header, err = sec.Bool("header", true); err != nil {
		return nil, err
	}
	if srv.headerTimeout, err = sec.Duration("header_timeout", 10*time.Second); err != nil {
		return nil, err
	}
	if srv.trustUser, err = sec.Bool("trust_user", false); err != nil {
		return nil, err
	}
	if srv.trustUser && !srv.header {
		return nil, fmt.Errorf("[server] trust_user needs header = yes")
	}
	if srv.maxSessions, err = sec.Int("max_sessions", 32); err != nil {
		return nil, err
	}
	if srv.maxSessions < 1 {
		return nil, fmt.Errorf("[server] max_sessions: %d", srv.maxSessions)
	}
	if !strings.HasPrefix(srv.listen, "/") && !strings.HasPrefix(srv.listen, "tcp:") {
		return nil, fmt.Errorf("[server] listen: %q (an absolute path, or tcp:host:port)", srv.listen)
	}
	return srv, nil
}
