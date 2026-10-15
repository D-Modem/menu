package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStripComment(t *testing.T) {
	for in, want := range map[string]string{
		"P ; PPP":                  "P ",
		"P # PPP":                  "P ",
		"a;b":                      "a;b",
		"echo a; echo b":           "echo a; echo b",
		`"a ; b" ; c`:              `"a ; b" `,
		`x \; y`:                   "x ; y",
		`x \# y`:                   "x # y",
		"; all comment":            "",
		"(&(uid={user})(#x))":      "(&(uid={user})(#x))",
		"exec pppd ipparam {user}": "exec pppd ipparam {user}",
	} {
		if got := stripComment(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// accessConf: a configuration with groups, item rules, guest, signup and
// caller ID rules
func accessConf(t *testing.T, dir, extra string) string {
	users := filepath.Join(dir, "users")
	if _, err := os.Stat(users); err != nil {
		os.WriteFile(users, []byte("alice:{PLAIN}secret\nbob:{PLAIN}secret\nlocked:!{PLAIN}secret\n"), 0o600)
	}
	p := filepath.Join(dir, "access.conf")
	os.WriteFile(p, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
%s
[file]
path = %s
[groups]
sysops = alice   ; the sysop
[guest]
enabled = yes
time_limit = 1h
[signup]
enabled = yes
pending = no
min_password = 6
[callerid]
deny = 5550000 1900*
[number 5551234]
user = alice
item = info
[number 5556666]
deny = yes
[menu]
prompt = "\r\nSelection ({time_left} min): "
[item info]
key = I
command = echo "INFO user=$MODEM_USER auth=$MODEM_AUTH left=$MODEM_TIME_LEFT"
after = menu
[item admin]
key = A
groups = sysops
command = echo ADMIN
after = menu
[item fast]
key = F
min_baud = 33600
command = echo FAST
after = menu
[item pub]
key = P
guest = yes
command = echo PUB
after = menu
[item local]
key = L
callerids = 555*
command = echo LOCAL
after = menu
[item nap]
key = N
command = exec sleep 30
`, filepath.Join(dir, "log"), extra, users)), 0o600)
	return p
}

func TestAccessRules(t *testing.T) {
	dir := t.TempDir()
	s, err := loadSettings(accessConf(t, dir, ""))
	if err != nil {
		t.Fatal(err)
	}
	names := func(sess *session) string {
		var n []string
		for _, it := range s.visible(sess) {
			n = append(n, it.name)
		}
		return strings.Join(n, " ")
	}
	for _, c := range []struct {
		user, auth, rate, callerID, want string
	}{
		{"alice", "file", "33600/31200", "5551111", "info admin fast pub local nap"},
		{"bob", "file", "28800/28800", "5551111", "info pub local nap"},
		{"bob", "file", "", "", "info pub nap"},
		{"guest", "guest", "33600/33600", "5551111", "pub"},
	} {
		sess := newSession(c.rate, "", c.callerID, time.Now())
		sess.User, sess.Auth = c.user, c.auth
		if got := names(sess); got != c.want {
			t.Errorf("%s %s %q %q: %q, want %q", c.user, c.auth, c.rate, c.callerID, got, c.want)
		}
	}
	for number, want := range map[string]bool{
		"5551111": true, "5550000": false, "19005551212": false, "5556666": false, "": true,
	} {
		if got := s.callerAllowed(number); got != want {
			t.Errorf("caller %q: %v, want %v", number, got, want)
		}
	}
	s.callerID.anonymous = false
	if s.callerAllowed("") {
		t.Error("anonymous = no: a call without caller ID taken")
	}
	s.callerID.allow = []string{"555*"}
	if s.callerAllowed("4441111") || !s.callerAllowed("5551111") {
		t.Error("allow = 555*")
	}
	if r := s.numberRule("5551234"); r == nil || r.user != "alice" || r.item != "info" {
		t.Errorf("number rule %+v", r)
	}

	// the users file: a locked account
	fa, _ := newFileAuth(&section{name: "file", vals: map[string]string{"path": filepath.Join(dir, "users")}})
	if err := fa.authenticate(context.Background(), "locked", "secret"); !errors.Is(err, errBadPassword) {
		t.Errorf("locked: %v", err)
	}

	// signup's names
	for name, taken := range map[string]bool{"carol": false, "alice": true, "ALICE": true, "guest": true,
		"new": true, "root": true, "bad name": true} {
		if got := s.nameTaken(name) != ""; got != taken {
			t.Errorf("name %q taken: %v, want %v", name, got, taken)
		}
	}
	if err := appendUser(filepath.Join(dir, "users"), "carol", "carol:x\n"); err != nil {
		t.Fatal(err)
	}
	if err := appendUser(filepath.Join(dir, "users"), "Carol", "Carol:x\n"); err == nil {
		t.Error("a second carol")
	}

	for _, bad := range []string{"[number 1[]\nuser = alice", "[number 555]\nitem = nothing",
		"[item x]\ngroups = nobody\ncommand = true"} {
		p := filepath.Join(dir, "bad.conf")
		os.WriteFile(p, []byte("[general]\nauth = file\n[file]\npath = "+filepath.Join(dir, "users")+"\n[item i]\ncommand = true\n"+bad+"\n"), 0o600)
		if _, err := loadSettings(p); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestUsage(t *testing.T) {
	dir := t.TempDir()
	if err := addUsage(dir, "a/b", "2026-10-12", 90*time.Second); err != nil {
		t.Fatal(err)
	}
	addUsage(dir, "a/b", "2026-10-12", 30*time.Second)
	if d, _ := usedToday(dir, "a/b", "2026-10-12"); d != 2*time.Minute {
		t.Errorf("used %v", d)
	}
	if d, _ := usedToday(dir, "a/b", "2026-10-13"); d != 0 {
		t.Errorf("the next day: %v", d)
	}
	addUsage(dir, "a/b", "2026-10-13", time.Minute)
	if d, _ := usedToday(dir, "a/b", "2026-10-13"); d != time.Minute {
		t.Errorf("the next day after use: %v", d)
	}

	s := &settings{limits: limitSettings{timeLimit: time.Hour, dailyLimit: 10 * time.Minute, stateDir: dir}}
	s.guest.timeLimit = 5 * time.Minute
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	sess := &session{User: "a/b", Auth: "file"}
	l, err := newLimiter(s, sess, now)
	if err != nil || !l.deadline.Equal(now.Add(9*time.Minute)) || l.timeLeft(now) != "9" {
		t.Errorf("limiter %v %v %q", err, l.deadline, l.timeLeft(now))
	}
	addUsage(dir, "a/b", "2026-10-13", 9*time.Minute)
	if _, err := newLimiter(s, sess, now); !errors.Is(err, errTimeLimit) {
		t.Errorf("used up: %v", err)
	}
	g, _ := newLimiter(s, &session{User: "guest", Auth: "guest"}, now)
	if !g.deadline.Equal(now.Add(5 * time.Minute)) {
		t.Errorf("guest deadline %v", g.deadline)
	}
}

// callAs: modemmenu on a pty with these call details
func callAs(t *testing.T, bin, conf string, env ...string) (*caller, *exec.Cmd) {
	m, s := testPty(t)
	t.Cleanup(func() { m.Close() })
	cmd := exec.Command(bin, "-config", conf)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return newCaller(t, m), cmd
}

func waitExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("modemmenu is still running")
	}
}

func TestAccessLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	conf := accessConf(t, dir, "time_limit = 1h")

	// bob at 28800: no admin, no fast; what is not shown cannot be chosen
	c, cmd := callAs(t, bin, conf, "DMODEM_RATE=28800/28800", "DMODEM_CALLERID=4441111")
	c.expect("login: ")
	c.send("bob\r")
	c.expect("Password: ")
	c.send("secret\r")
	menu := c.expect("Selection (59 min): ")
	if strings.Contains(menu, "admin") || strings.Contains(menu, "fast") || strings.Contains(menu, "local") ||
		!strings.Contains(menu, "[I] info") || !strings.Contains(menu, "[N] nap") {
		t.Errorf("bob's menu:\n%s", menu)
	}
	c.send("admin\r")
	c.expect("Unknown selection")
	c.send("i\r")
	c.expect("INFO user=bob auth=file left=59")
	c.send("g\r")
	c.expect("Goodbye")
	waitExit(t, cmd)

	// a guest: no password, the guest items only
	c, cmd = callAs(t, bin, conf, "DMODEM_RATE=33600/33600")
	c.expect("login: ")
	c.send("guest\r")
	menu = c.expect("Selection (59 min): ")
	if strings.Contains(menu, "Password") || strings.Contains(menu, "info") || !strings.Contains(menu, "[P] pub") {
		t.Errorf("the guest's menu:\n%s", menu)
	}
	c.send("pub\r")
	c.expect("PUB")
	c.send("g\r")
	waitExit(t, cmd)

	// signup: a new account, logged in with it at once
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("new\r")
	c.expect("User name: ")
	c.send("alice\r")
	c.expect("taken")
	c.expect("User name: ")
	c.send("carol\r")
	c.expect("Password: ")
	c.send("abc\r")
	c.expect("At least 6")
	c.expect("Password: ")
	c.send("carol-pw\r")
	c.expect("Again: ")
	c.send("carol-pw\r")
	c.expect("Selection")
	c.send("info\r")
	c.expect("INFO user=carol auth=file")
	c.send("g\r")
	waitExit(t, cmd)
	// and carol logs in with it
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("carol\r")
	c.expect("Password: ")
	c.send("carol-pw\r")
	c.expect("Selection")
	c.send("g\r")
	waitExit(t, cmd)

	// a denied number: no banner, no login
	c, cmd = callAs(t, bin, conf, "DMODEM_CALLERID=19005551212")
	c.expect("does not take your call")
	waitExit(t, cmd)

	// a known number: alice without a password, her item at once
	c, cmd = callAs(t, bin, conf, "DMODEM_CALLERID=5551234", "DMODEM_RATE=33600/33600")
	c.expect("INFO user=alice auth=callerid")
	menu = c.expect("Selection")
	if strings.Contains(menu, "login:") || !strings.Contains(menu, "[A] admin") || !strings.Contains(menu, "[L] local") {
		t.Errorf("alice's menu:\n%s", menu)
	}
	c.send("g\r")
	waitExit(t, cmd)

	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	for _, w := range []string{"login user=guest auth=guest", "signup user=carol pending=false",
		"refused by caller ID", "login user=alice auth=callerid"} {
		if !strings.Contains(string(log), w) {
			t.Errorf("no %q in the log:\n%s", w, log)
		}
	}
	if users, _ := os.ReadFile(filepath.Join(dir, "users")); !strings.Contains(string(users), "carol:$2a$") {
		t.Errorf("users file:\n%s", users)
	}
}

func TestTimeLimits(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	conf := accessConf(t, dir, "daily_limit = 3s\nstate_dir = "+filepath.Join(dir, "state"))

	// the selection is hung up when the day's time is up
	c, cmd := callAs(t, bin, conf)
	c.expect("login: ")
	c.send("bob\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection (0 min): ")
	start := time.Now()
	c.send("nap\r")
	c.expect("Your time is up.")
	if d := time.Since(start); d > 6*time.Second {
		t.Errorf("hung up after %v", d)
	}
	waitExit(t, cmd)

	// and the next call of the day is refused
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("bob\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("used your time for today")
	waitExit(t, cmd)

	// a pending signup: locked until the sysop unlocks it
	p := filepath.Join(dir, "pending.conf")
	b, _ := os.ReadFile(conf)
	os.WriteFile(p, []byte(strings.Replace(string(b), "pending = no", "pending = yes", 1)), 0o600)
	c, cmd = callAs(t, bin, p)
	c.expect("login: ")
	c.send("new\r")
	c.expect("User name: ")
	c.send("dave\r")
	c.expect("Password: ")
	c.send("dave-pw\r")
	c.expect("Again: ")
	c.send("dave-pw\r")
	c.expect("waits for the sysop")
	waitExit(t, cmd)
	if users, _ := os.ReadFile(filepath.Join(dir, "users")); !strings.Contains(string(users), "dave:!$2a$") {
		t.Errorf("users file:\n%s", users)
	}
}

// the time_warning on the line while a selection runs, and not for an
// item with time_warning = no
func TestTimeWarning(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("bob:{PLAIN}secret\n"), 0o600)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = none
auth = file
time_limit = 4s
time_warning = 2s
time_warning_message = "\r\n*** {time_left} minute(s) left, {user} ***\r\n"
[file]
path = %s
[item nap]
command = exec sleep 30
[item quiet]
time_warning = no
command = exec sleep 30
`, users)), 0o600)
	for _, c := range []struct {
		item string
		warn bool
	}{{"nap", true}, {"quiet", false}} {
		cl, cmd := callAs(t, bin, conf)
		cl.expect("login: ")
		cl.send("bob\r")
		cl.expect("Password: ")
		cl.send("secret\r")
		cl.expect("Selection: ")
		cl.send(c.item + "\r")
		got := cl.expect("Your time is up.")
		if warned := strings.Contains(got, "*** 0 minute(s) left, bob ***"); warned != c.warn {
			t.Errorf("%s: warned %v, want %v:\n%q", c.item, warned, c.warn, got)
		}
		waitExit(t, cmd)
	}
}
