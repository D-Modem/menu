package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sys/unix"
)

// errPending: the new account waits for the sysop
var errPending = errors.New("account pending approval")

// signup: a new account in the users file, the caller logged in with it
// (or errPending with signup's pending = yes)
func signup(t *term, s *settings, sess *session, deadline time.Time) error {
	su := s.signup
	if err := t.write(su.message); err != nil {
		return err
	}
	var name string
	for try := 0; ; try++ {
		if try == 3 {
			return errors.New("signup: no name taken")
		}
		if err := t.write("\r\nUser name: "); err != nil {
			return err
		}
		n, err := t.readLine(deadline, s.maxInput, true, "")
		if err != nil {
			return err
		}
		n = strings.TrimSpace(n)
		if why := s.nameTaken(n); why != "" {
			t.write(why + "\r\n")
			continue
		}
		name = n
		break
	}
	var pw string
	for try := 0; ; try++ {
		if try == 3 {
			return errors.New("signup: no password")
		}
		if err := t.write("Password: "); err != nil {
			return err
		}
		p, err := t.readLine(deadline, s.maxInput*2, false, s.mask)
		if err != nil {
			return err
		}
		if len(p) < su.minPassword {
			t.write(fmt.Sprintf("At least %d characters, please.\r\n", su.minPassword))
			continue
		}
		if err := t.write("Again: "); err != nil {
			return err
		}
		again, err := t.readLine(deadline, s.maxInput*2, false, s.mask)
		if err != nil {
			return err
		}
		if again != p {
			t.write("They differ.\r\n")
			continue
		}
		pw = p
		break
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	lock := ""
	if su.pending {
		lock = "!"
	}
	note := fmt.Sprintf("signup %s %s", time.Now().Format("2006-01-02 15:04"), sess.CallerID)
	note = strings.Map(func(r rune) rune {
		if r == ':' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, note)
	if err := appendUser(su.file, name, name+":"+lock+string(hash)+":"+note+"\n"); err != nil {
		logf("signup %s: %v", name, err)
		t.write("\r\nSorry, no new accounts now.\r\n")
		return err
	}
	logf("signup user=%s pending=%v %v", name, su.pending, sess)
	if su.pending {
		t.write("\r\nThank you.  Your account waits for the sysop's approval.\r\n")
		return errPending
	}
	sess.User, sess.Password, sess.Auth = name, pw, "file"
	return nil
}

// nameTaken: why a new user may not have name, or ""
func (s *settings) nameTaken(name string) string {
	switch {
	case !s.userPattern.MatchString(name):
		return "Letters, digits and . _ @ - only, please."
	case strings.EqualFold(name, s.signup.name) || strings.EqualFold(name, s.guest.name) ||
		strings.EqualFold(name, "root"):
		return "That name is taken."
	}
	if has, err := fileHasUser(s.signup.file, name); err != nil || has {
		return "That name is taken."
	}
	// a system account's name would let the new user log in to a
	// selection run_as that account
	if _, err := lookupNSS(name); err == nil {
		return "That name is taken."
	}
	return ""
}

func fileHasUser(path, name string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	return scanUsers(f, name)
}

func scanUsers(f *os.File, name string) (bool, error) {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if n, _, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":"); ok && strings.EqualFold(n, name) {
			return true, nil
		}
	}
	return false, sc.Err()
}

// appendUser: line at the end of the users file, unless name is there
// already (looked at again under the lock: two signups at once)
func appendUser(path, name, line string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	if has, err := scanUsers(f, name); err != nil || has {
		if err == nil {
			err = fmt.Errorf("%s is taken", name)
		}
		return err
	}
	// a file without a line end at its end
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err == nil && last[0] != '\n' {
			line = "\n" + line
		}
	}
	_, err = f.WriteString(line)
	return err
}
