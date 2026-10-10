package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

type general struct {
	log            string
	banner         string
	loginPrompt    string
	passwordPrompt string
	welcome        string
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
	name        string
	key         string
	label       string
	mode        string // exec, tcp, rlogin, telnet
	command     string
	host        string
	port        int
	send        string
	rloginLocal string
	rloginUser  string
	rloginTerm  string
	terminal    string
	runAs       string
	tty         string // raw, sane
	password    bool
	sessionFile bool
	after       string // hangup, menu
	timeout     time.Duration
}

type settings struct {
	general
	menuTitle  string
	menuPrompt string
	hangupKey  string
	items      []*menuItem
	backends   []authenticator
}

func loadSettings(path string) (*settings, error) {
	c, err := parseINI(path)
	if err != nil {
		return nil, err
	}
	s := &settings{}
	g := c.section("general")
	s.log = g.get("log", "syslog")
	s.banner = unescape(g.get("banner", `\r\nD-Modem\r\n`))
	if f := g.get("banner_file", ""); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("[general] banner_file: %v", err)
		}
		s.banner = crlf(string(b))
	}
	s.loginPrompt = unescape(g.get("login_prompt", `\r\nlogin: `))
	s.passwordPrompt = unescape(g.get("password_prompt", "Password: "))
	s.welcome = unescape(g.get("welcome", `\r\nWelcome, {user}.\r\n`))
	s.goodbye = unescape(g.get("goodbye", `\r\nGoodbye.\r\n`))
	s.mask = g.get("password_echo", "*")
	if s.mask == "none" {
		s.mask = ""
	}
	if s.attempts, err = g.int("attempts", 3); err != nil {
		return nil, err
	}
	if s.maxInput, err = g.int("max_input", 64); err != nil {
		return nil, err
	}
	if s.failDelay, err = g.duration("fail_delay", 3*time.Second); err != nil {
		return nil, err
	}
	if s.loginTimeout, err = g.duration("login_timeout", 90*time.Second); err != nil {
		return nil, err
	}
	if s.idleTimeout, err = g.duration("idle_timeout", 5*time.Minute); err != nil {
		return nil, err
	}
	pat := g.get("user_pattern", `^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)
	if s.userPattern, err = regexp.Compile(pat); err != nil {
		return nil, fmt.Errorf("[general] user_pattern: %v", err)
	}
	s.auth = fields(strings.ToLower(g.get("auth", "file")))
	if len(s.auth) == 0 {
		return nil, fmt.Errorf("[general] auth: no authentication backend")
	}
	for _, name := range s.auth {
		var a authenticator
		switch name {
		case "file":
			a, err = newFileAuth(c.section("file"))
		case "ldap":
			a, err = newLDAPAuth(c.section("ldap"))
		case "sql":
			a, err = newSQLAuth(c.section("sql"))
		case "system":
			a, err = newSystemAuth(c.section("system"))
		default:
			err = fmt.Errorf("[general] auth: unknown backend %q (file, ldap, sql, system)", name)
		}
		if err != nil {
			return nil, err
		}
		s.backends = append(s.backends, a)
	}

	m := c.section("menu")
	s.menuTitle = unescape(m.get("title", `\r\nMain menu\r\n`))
	s.menuPrompt = unescape(m.get("prompt", `\r\nSelection: `))
	s.hangupKey = strings.ToUpper(m.get("hangup_key", "G"))
	var names []string
	if m.has("items") {
		names = fields(m.get("items", ""))
	} else {
		for _, sec := range c.sections {
			if strings.HasPrefix(sec.name, "item ") {
				names = append(names, strings.TrimPrefix(sec.name, "item "))
			}
		}
	}
	keys := map[string]string{s.hangupKey: "hangup_key"}
	for _, name := range names {
		sec, ok := c.byName["item "+name]
		if !ok {
			return nil, fmt.Errorf("[menu] items: no [item %s]", name)
		}
		it, err := loadItem(name, sec)
		if err != nil {
			return nil, err
		}
		if prev, dup := keys[it.key]; dup {
			return nil, fmt.Errorf("[item %s] key %s: also the key of %s", name, it.key, prev)
		}
		keys[it.key] = "[item " + name + "]"
		s.items = append(s.items, it)
	}
	if len(s.items) == 0 {
		return nil, fmt.Errorf("%s: no [item ...] in the menu", path)
	}
	return s, nil
}

func loadItem(name string, sec *section) (*menuItem, error) {
	var err error
	it := &menuItem{name: name}
	it.key = strings.ToUpper(sec.get("key", name[:1]))
	if len(it.key) != 1 {
		return nil, fmt.Errorf("[item %s] key: one character", name)
	}
	it.label = sec.get("label", name)
	it.mode = strings.ToLower(sec.get("mode", "exec"))
	it.command = sec.get("command", "")
	it.host = sec.get("host", "127.0.0.1")
	it.send = sec.get("send", "")
	it.rloginLocal = sec.get("rlogin_client_user", "{password}")
	it.rloginUser = sec.get("rlogin_server_user", "{user}")
	it.rloginTerm = sec.get("rlogin_terminal", "ansi/{baud}")
	it.terminal = sec.get("terminal", "ANSI")
	it.runAs = sec.get("run_as", "")
	it.tty = strings.ToLower(sec.get("tty", "raw"))
	it.after = strings.ToLower(sec.get("after", "hangup"))
	if it.password, err = sec.bool("pass_password", true); err != nil {
		return nil, err
	}
	if it.sessionFile, err = sec.bool("session_file", false); err != nil {
		return nil, err
	}
	if it.timeout, err = sec.duration("connect_timeout", 10*time.Second); err != nil {
		return nil, err
	}
	defPort := map[string]int{"tcp": 23, "telnet": 23, "rlogin": 513}[it.mode]
	if it.port, err = sec.int("port", defPort); err != nil {
		return nil, err
	}
	switch it.mode {
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
	default:
		return nil, fmt.Errorf("[item %s] mode: %q (exec, tcp, telnet or rlogin)", name, it.mode)
	}
	if it.tty != "raw" && it.tty != "sane" {
		return nil, fmt.Errorf("[item %s] tty: %q (raw or sane)", name, it.tty)
	}
	if it.after != "hangup" && it.after != "menu" {
		return nil, fmt.Errorf("[item %s] after: %q (hangup or menu)", name, it.after)
	}
	return it, nil
}

// crlf: the line ends of a text file as the terminal wants them
func crlf(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
