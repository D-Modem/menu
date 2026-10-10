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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	xterm "golang.org/x/term"
)

var logger = log.New(io.Discard, "", 0)

func logf(format string, args ...any) { logger.Printf(format, args...) }

func main() {
	config := flag.String("config", "/etc/asterisk/modemmenu.conf", "the configuration `file`")
	rate := flag.String("rate", os.Getenv("DMODEM_RATE"), "the connect `rate`, tx/rx bit/s (default $DMODEM_RATE)")
	dialed := flag.String("dialed", os.Getenv("DMODEM_DIALED"), "the dialed `number` (default $DMODEM_DIALED)")
	callerID := flag.String("callerid", os.Getenv("DMODEM_CALLERID"), "the caller's `number` (default $DMODEM_CALLERID)")
	check := flag.Bool("check", false, "check the configuration and exit")
	hash := flag.Bool("hash", false, "read a password, print its bcrypt hash for the users file and exit")
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
	sess := newSession(*rate, *dialed, *callerID, time.Now())
	t := newTerm(os.Stdin, os.Stdout)
	code := run(t, s, sess)
	t.close()
	os.Exit(code)
}

// terminated: SIGTERM or SIGHUP came
var terminated atomic.Bool

func init() {
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
				os.Exit(1)
			}
			// the program ends, its session file is removed, then us
			p.Signal(sig)
		}
	}()
}

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
	if t.write(expand(s.banner, sess.vars(false), nil)) != nil {
		return 1
	}
	if err := login(t, s, sess); err != nil {
		if !errors.Is(err, errHangup) {
			t.write("\r\n")
		}
		logf("no login: %v (%v)", err, sess)
		return 1
	}
	t.write(expand(s.welcome, sess.vars(false), nil))
	t.write(motd(s, sess))
	err := menu(t, s, sess)
	switch {
	case err == nil:
		t.write(expand(s.goodbye, sess.vars(false), nil))
	case errors.Is(err, errTimeout):
		t.write("\r\nIdle too long.\r\n")
	}
	logf("logout user=%s: %v", sess.User, describe(err))
	return 0
}

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
	for try := 1; try <= s.attempts; try++ {
		if err := t.write(s.loginPrompt); err != nil {
			return err
		}
		name, err := t.readLine(deadline, s.maxInput, true, "")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			try--
			continue
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

// menu until the caller hangs up, says goodbye or a selection ends with
// after = hangup
func menu(t *term, s *settings, sess *session) error {
	redraw := true
	for {
		if redraw {
			var b strings.Builder
			b.WriteString(expand(s.menuTitle, sess.vars(false), nil))
			width := len(s.hangup[0])
			for _, it := range s.items {
				width = max(width, len(it.key))
			}
			for _, it := range s.items {
				fmt.Fprintf(&b, "\r\n  [%s]%*s %s", it.key, width-len(it.key), "", it.label)
			}
			fmt.Fprintf(&b, "\r\n  [%s]%*s Goodbye (hang up)\r\n", s.hangup[0], width-len(s.hangup[0]), "")
			if err := t.write(b.String()); err != nil {
				return err
			}
		}
		if err := t.write(s.menuPrompt); err != nil {
			return err
		}

		// a line (the key, the item's name or an alias: P, ppp), or with
		// hotkeys one key that selects at once
		var choice string
		var err error
		if s.hotkeys {
			if choice, err = t.readKey(time.Now().Add(s.idleTimeout)); err == nil {
				t.write(choice + "\r\n")
			}
		} else {
			choice, err = t.readLine(time.Now().Add(s.idleTimeout), s.maxInput, true, "")
		}
		choice = strings.ToUpper(strings.TrimSpace(choice))
		if err == io.EOF || contains(s.hangup, choice) {
			return nil
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
		it := s.selects(choice)
		if it == nil {
			t.write("Unknown selection; ? shows the menu.\r\n")
			continue
		}
		// what the caller types from here on is the selection's: nothing
		// is read ahead

		sess.Selection = it.name
		logf("start %s user=%s mode=%s", it.name, sess.User, it.mode)
		err = launch(t, it, sess, logf)
		if errors.Is(err, errHangup) || terminated.Load() {
			return errHangup
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
	for _, it := range s.items {
		if contains(it.choices, choice) {
			return it
		}
	}
	return nil
}
