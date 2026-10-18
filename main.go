// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

// modemmenu: the login and menu of a dial-up line.  app_dmodem (or any
// getty) runs it on the caller's line once the modems are connected; it
// asks for a user name and password, checks them with a text file, LDAP
// or an SQL database, and offers a menu (a BBS, PPP, a shell...) whose
// selection it starts with the caller's credentials and the call's
// details: dialed number, rate, caller ID and the time of the connection.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	xterm "golang.org/x/term"
)

var logger = log.New(io.Discard, "", 0)

// logf: a line in the log ([general] log)
func logf(format string, args ...any) { logger.Printf(format, args...) }

// main: the flags; then -hash, -check, -authd or -serve, or a call on
// stdin and stdout
func main() {
	config := flag.String("config", "/etc/asterisk/modemmenu.conf", "the configuration `file`")
	rate := flag.String("rate", os.Getenv("DMODEM_RATE"), "the connect `rate`, tx/rx bit/s (default $DMODEM_RATE)")
	dialed := flag.String("dialed", os.Getenv("DMODEM_DIALED"), "the dialed `number` (default $DMODEM_DIALED)")
	callerID := flag.String("callerid", os.Getenv("DMODEM_CALLERID"), "the caller's `number` (default $DMODEM_CALLERID)")
	check := flag.Bool("check", false, "check the configuration and exit")
	hash := flag.Bool("hash", false, "read a password, print its bcrypt hash for the users file and exit")
	serveFlag := flag.Bool("serve", false, "serve sessions on the [server] listen socket")
	listenAt := flag.String("listen", "", "with -serve: listen on this unix socket `path` (or tcp:host:port) instead of [server] listen")
	upstreamFlag := flag.Bool("upstream", false, "a session of -serve with trust_user (internal)")
	authdFlag := flag.Bool("authd", false, "answer the [authd] auth socket alone (with -serve it is answered too)")
	flag.Parse()

	if *hash {
		os.Exit(printHash())
	}
	s, err := loadSettings(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		os.Exit(2)
	}
	if *check {
		fmt.Printf("%s: %d menu items, auth %s\n", *config, len(s.items), strings.Join(s.auth, ", "))
		return
	}
	if err := openLog(s.log); err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu: log:", err)
	}
	if *authdFlag {
		os.Exit(authd(s))
	}
	if *serveFlag {
		if *listenAt != "" {
			s.server.listen = *listenAt
		}
		os.Exit(serve(s, *config))
	}
	handleSignals()
	connected := time.Now()
	if e, err := strconv.ParseInt(os.Getenv("DMODEM_CONNECTED"), 10, 64); err == nil && e > 0 {
		connected = time.Unix(e, 0)
	}
	sess := newSession(*rate, *dialed, *callerID, connected)
	if upstream(sess, *upstreamFlag) && !s.userPattern.MatchString(sess.User) {
		logf("upstream user %q: not a name we take", sess.User)
		sess.User, sess.Password, sess.Auth = "", "", ""
	}
	t := newTerm(os.Stdin, os.Stdout)
	code := run(t, s, sess)
	t.close()
	os.Exit(code)
}

// terminated: SIGTERM or SIGHUP came
var terminated atomic.Bool

// handleSignals: a hangup (SIGHUP, SIGTERM) passed to the selection
// running, or the end of modemmenu; ^C and ^\ dropped
func handleSignals() {
	// a selection on a cooked line gets ^C and ^\ from the terminal, and
	// so would we: caught (not ignored, which the selection would inherit)
	// and dropped.  A shell that took the terminal for a job of its own
	// gives it back while we are in the background: no stop for that
	signal.Ignore(syscall.SIGTTOU)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		for sig := range ch {
			if sig == syscall.SIGINT || sig == syscall.SIGQUIT {
				continue
			}
			terminated.Store(true)
			childMu.Lock()
			p := child
			childMu.Unlock()
			if p == nil {
				logf("%v: hung up", sig)
				runAtExit()
				os.Exit(1)
			}
			// the program ends, its session file is removed, then us
			p.Signal(sig)
		}
	}()
}

// openLog: the log: none, syslog, or a file
func openLog(dest string) error {
	switch dest {
	case "", "none":
		return nil
	case "syslog":
		w, err := syslog.New(syslog.LOG_AUTH|syslog.LOG_INFO, "modemmenu")
		if err != nil {
			return err
		}
		logger = log.New(w, "", 0)
	default:
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o640)
		if err != nil {
			return err
		}
		logger = log.New(f, fmt.Sprintf("modemmenu[%d]: ", os.Getpid()), log.LstdFlags)
	}
	return nil
}

// printHash: -hash: a password read (without echo on a terminal) and
// its bcrypt hash printed; the exit status
func printHash() int {
	fmt.Fprint(os.Stderr, "Password: ")
	var pw []byte
	var err error
	if xterm.IsTerminal(int(os.Stdin.Fd())) {
		pw, err = xterm.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		var line string
		line, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err == io.EOF && line != "" {
			err = nil
		}
		pw = []byte(strings.TrimRight(line, "\r\n"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	h, err := bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	fmt.Println(string(h))
	return 0
}

// the call: who and where from, for the log
func (s *session) String() string {
	return fmt.Sprintf("callerid=%q dialed=%q rate=%q channel=%q", s.CallerID, s.Dialed, s.Rate, s.Channel)
}

// run the login and the menu: the exit status
func run(t *term, s *settings, sess *session) int {
	logf("connect %v", sess)
	if !s.callerAllowed(sess.CallerID) {
		logf("refused by caller ID %v", sess)
		t.write(s.callerID.denyMessage)
		return 1
	}
	// what the terminal is, before anything is written for it
	if err := detectTerminal(t, s, sess); err != nil {
		logf("no login: %v (%v)", err, sess)
		return 1
	}
	if sess.Terminal != "" {
		logf("terminal %s %v", sess.describeTerminal(), sess)
	}
	if t.write(expand(s.banner, sess.vars(false), nil)) != nil {
		return 1
	}
	// the connect rate, as BBSs greeted their callers (not when it is
	// not known: no "Connected at ." on a getty without -rate)
	if s.connectMessage != "" && sess.Speed() != "" {
		t.write(expand(s.connectMessage, sess.vars(false), nil))
	}
	rule := s.numberRule(sess.CallerID)
	switch {
	case sess.User != "":
		// an upstream menu logged the caller in
		logf("login user=%s auth=%s %v", sess.User, sess.Auth, sess)
	case rule != nil && rule.user != "":
		// the caller's number logs them in: [number ...] user
		sess.User, sess.Auth = rule.user, "callerid"
		logf("login user=%s auth=callerid %v", sess.User, sess)
	default:
		err := login(t, s, sess)
		if errors.Is(err, errAutoPPP) {
			return autoPPP(t, s, sess)
		}
		if err != nil {
			if !errors.Is(err, errHangup) {
				t.write("\r\n")
			}
			logf("no login: %v (%v)", err, sess)
			return 1
		}
	}

	lim, err := newLimiter(s, sess, time.Now())
	if err != nil {
		logf("user=%s: no time left today", sess.User)
		t.write("\r\nYou have used your time for today.  Call again tomorrow.\r\n")
		return 0
	}
	setAtExit(lim.done)
	defer lim.done()
	if s.hasCallers() {
		if err := recordCaller(s.limits.stateDir, sess, time.Now()); err != nil {
			logf("callers: %v", err)
		}
	}
	lim.mark(sess, time.Now())

	t.write(expand(s.welcome, sess.vars(false), nil))
	t.write(motd(s, sess))
	var first *menuItem
	if rule != nil && rule.item != "" {
		first = s.selects(strings.ToUpper(rule.item))
		if first != nil && !s.allows(first, sess) {
			first = nil
		}
	}
	err = menu(t, s, sess, lim, first)
	switch {
	case err == nil:
		t.write(expand(s.goodbye, sess.vars(false), nil))
	case errors.Is(err, errTimeLimit):
		t.write("\r\n\r\nYour time is up." + expand(s.goodbye, sess.vars(false), nil))
	case errors.Is(err, errTimeout):
		t.write("\r\nIdle too long.\r\n")
	}
	logf("logout user=%s: %v", sess.User, describe(err))
	return 0
}

// describe: an error for the log, "done" for none
func describe(err error) string {
	if err == nil {
		return "done"
	}
	return err.Error()
}

// login: the user name and password until one is right, attempts tries
// or the login timeout
func login(t *term, s *settings, sess *session) error {
	deadline := time.Now().Add(s.loginTimeout)
	// a terminal not known by the end is not a Commodore
	defer func() { t.watchPETSCII = false }()
	for try := 1; try <= s.attempts; try++ {
		if err := t.write(s.loginPrompt); err != nil {
			return err
		}
		if s.autoppp.enabled {
			// a dialer that starts PPP at once: no name, a PPP frame
			ppp, err := sniffPPP(t, deadline)
			if err != nil {
				return err
			}
			if ppp {
				return errAutoPPP
			}
		}
		name, err := t.readLine(deadline, s.maxInput, true, "")
		if errors.Is(err, errPETSCII) {
			// a Commodore's key: the prompt again, for it
			becomePETSCII(t, sess)
			try--
			continue
		}
		if err != nil {
			return err
		}
		if t.watchPETSCII && name != "" {
			if name, err = settlePETSCII(t, s, sess, name, deadline); err != nil {
				return err
			}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			try--
			continue
		}
		if s.guest.enabled && strings.EqualFold(name, s.guest.name) {
			// no password for a guest
			sess.User, sess.Auth = s.guest.name, "guest"
			logf("login user=%s auth=guest %v", sess.User, sess)
			return nil
		}
		if s.signup.enabled && strings.EqualFold(name, s.signup.name) {
			return signup(t, s, sess, deadline)
		}
		if err := t.write(s.passwordPrompt); err != nil {
			return err
		}
		pw, err := t.readLine(deadline, s.maxInput*2, false, s.mask)
		if err != nil {
			return err
		}
		auth := ""
		if s.userPattern.MatchString(name) {
			ctx, cancel := context.WithDeadline(context.Background(), deadline.Add(10*time.Second))
			auth, err = authenticate(ctx, s.backends, name, pw, logf)
			cancel()
		} else {
			// not a name we take: the same answer, after the same time
			burn(pw)
			err = errNoUser
		}
		if err == nil {
			sess.User, sess.Password, sess.Auth = name, pw, auth
			logf("login user=%s auth=%s %v", name, auth, sess)
			return nil
		}
		logf("failed login user=%q (%v) %v", name, err, sess)
		time.Sleep(s.failDelay)
		if err := t.write("\r\nLogin incorrect\r\n"); err != nil {
			return err
		}
		if terminated.Load() {
			return errHangup
		}
	}
	return errors.New("too many attempts")
}

// motd: the message of the day, motd then motd_file (read at each login,
// so that edits show at once), {placeholders} expanded
func motd(s *settings, sess *session) string {
	m := s.motd
	if s.motdFile != "" {
		b, err := os.ReadFile(s.motdFile)
		if err != nil {
			logf("motd_file: %v", err)
		} else {
			m += crlf(string(b))
		}
	}
	return expand(m, sess.vars(false), nil)
}

// menu until the caller hangs up, says goodbye, the time is up or a
// selection ends with after = hangup; first (if not nil) starts at once
func menu(t *term, s *settings, sess *session, lim *limiter, first *menuItem) error {
	items := s.visible(sess)
	redraw := first == nil
	for {
		lim.mark(sess, time.Now())
		left, limited := lim.left(time.Now())
		if limited && left <= 0 {
			return errTimeLimit
		}
		idle := time.Now().Add(s.idleTimeout)
		if limited && lim.deadline.Before(idle) {
			idle = lim.deadline
		}
		if redraw && first == nil {
			var b strings.Builder
			b.WriteString(expand(s.menuTitle, sess.vars(false), nil))
			width := len(s.hangup[0])
			for _, it := range items {
				width = max(width, len(it.key))
			}
			for _, it := range items {
				fmt.Fprintf(&b, "\r\n  [%s]%*s %s", it.key, width-len(it.key), "", it.label)
			}
			fmt.Fprintf(&b, "\r\n  [%s]%*s Goodbye (hang up)\r\n", s.hangup[0], width-len(s.hangup[0]), "")
			if err := t.write(b.String()); err != nil {
				return err
			}
		}
		it := first
		first = nil
		if it == nil {
			if err := t.write(expand(s.menuPrompt, sess.vars(false), nil)); err != nil {
				return err
			}

			// a line (the key, the item's name or an alias: P, ppp), or
			// with hotkeys one key that selects at once
			var choice string
			var err error
			if s.hotkeys {
				if choice, err = t.readKey(idle); err == nil {
					t.write(choice + "\r\n")
				}
			} else {
				choice, err = t.readLine(idle, s.maxInput, true, "")
			}
			choice = strings.ToUpper(strings.TrimSpace(choice))
			if err == io.EOF || contains(s.hangup, choice) {
				return nil
			}
			if errors.Is(err, errTimeout) && limited && !time.Now().Before(lim.deadline) {
				return errTimeLimit
			}
			if err != nil {
				return err
			}
			redraw = false
			if choice == "" {
				continue
			}
			if choice == "?" || choice == "MENU" {
				redraw = true
				continue
			}
			// what the caller may not use, it cannot tell from what is
			// not there
			if it = selectFrom(items, choice); it == nil {
				t.write("Unknown selection; ? shows the menu.\r\n")
				continue
			}
		}
		// what the caller types from here on is the selection's: nothing
		// is read ahead

		sess.Selection = it.name
		lim.mark(sess, time.Now())
		logf("start %s user=%s mode=%s", it.name, sess.User, it.mode)
		err := launch(t, s, it, sess, lim.deadline, logf)
		if errors.Is(err, errHangup) || terminated.Load() {
			return errHangup
		}
		if errors.Is(err, errTimeLimit) {
			logf("%s: time limit", it.name)
			return errTimeLimit
		}
		redraw = true
		if err != nil {
			logf("%s: %v", it.name, err)
			t.write("\r\n\r\n" + it.label + " is not available now.\r\n")
			continue
		}
		logf("end %s user=%s", it.name, sess.User)
		if it.after == "hangup" {
			return nil
		}
	}
}

// selects: the item a choice (upper case) selects
func (s *settings) selects(choice string) *menuItem {
	return selectFrom(s.items, choice)
}

// selectFrom: the item of items a choice (upper case) selects, or nil
func selectFrom(items []*menuItem, choice string) *menuItem {
	for _, it := range items {
		if contains(it.choices, choice) {
			return it
		}
	}
	return nil
}
