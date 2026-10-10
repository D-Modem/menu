package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GehirnInc/crypt"
	_ "github.com/GehirnInc/crypt/md5_crypt"
	_ "github.com/GehirnInc/crypt/sha256_crypt"
	_ "github.com/GehirnInc/crypt/sha512_crypt"
	"github.com/openwall/yescrypt-go"
)

// systemAuth: the accounts of the system, by their password in
// /etc/shadow (method = shadow) or by PAM (method = pam, in a build with
// -tags pam).  Only accounts with a uid of at least min_uid (not root,
// unless allow_root), in one of groups if it is set, may log in.
type systemAuth struct {
	method     string
	service    string
	passwdFile string
	shadowFile string
	groupFile  string
	minUID     int
	allowRoot  bool
	groups     []string
	now        func() time.Time
}

func newSystemAuth(s *section) (authenticator, error) {
	var err error
	a := &systemAuth{
		method:     strings.ToLower(s.get("method", "shadow")),
		service:    s.get("pam_service", "modemmenu"),
		passwdFile: s.get("passwd_file", "/etc/passwd"),
		shadowFile: s.get("shadow_file", "/etc/shadow"),
		groupFile:  s.get("group_file", "/etc/group"),
		groups:     fields(s.get("groups", "")),
		now:        time.Now,
	}
	if a.minUID, err = s.int("min_uid", 1000); err != nil {
		return nil, err
	}
	if a.allowRoot, err = s.bool("allow_root", false); err != nil {
		return nil, err
	}
	switch a.method {
	case "shadow":
		// not readable here is an error at each login (logged), not now:
		// -check runs as whoever runs it
	case "pam":
		if !havePAM {
			return nil, fmt.Errorf("[system] method = pam: this modemmenu is built without PAM (go build -tags pam)")
		}
	default:
		return nil, fmt.Errorf("[system] method: %q (shadow or pam)", a.method)
	}
	return a, nil
}

func (a *systemAuth) name() string { return "system" }

// account: a user of the passwd file
type account struct {
	name, passwd string
	uid, gid     int
}

func (a *systemAuth) authenticate(ctx context.Context, user, password string) error {
	acct, err := a.lookup(user)
	if err != nil {
		return err
	}
	if err := a.allowed(acct); err != nil {
		return err
	}
	if a.method == "pam" {
		return pamAuthenticate(a.service, user, password)
	}
	return a.checkShadow(acct, password)
}

// lookup the user in the passwd file (shadow) or the name service (pam,
// which also knows accounts of LDAP, NIS, sssd...)
func (a *systemAuth) lookup(user string) (*account, error) {
	if a.method == "pam" {
		return lookupNSS(user)
	}
	var acct *account
	err := scanColon(a.passwdFile, func(f []string) bool {
		if len(f) < 7 || f[0] != user {
			return false
		}
		uid, err1 := strconv.Atoi(f[2])
		gid, err2 := strconv.Atoi(f[3])
		if err1 == nil && err2 == nil {
			acct = &account{name: f[0], passwd: f[1], uid: uid, gid: gid}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errNoUser
	}
	return acct, nil
}

// allowed: an account that may dial in.  The others are not this
// backend's users: the next one is asked
func (a *systemAuth) allowed(acct *account) error {
	if acct.uid == 0 && !a.allowRoot {
		return fmt.Errorf("%w: %s is root", errNoUser, acct.name)
	}
	if acct.uid != 0 && acct.uid < a.minUID {
		return fmt.Errorf("%w: %s has uid %d < min_uid %d", errNoUser, acct.name, acct.uid, a.minUID)
	}
	if len(a.groups) == 0 {
		return nil
	}
	in, err := a.inGroups(acct)
	if err != nil {
		return err
	}
	if !in {
		return fmt.Errorf("%w: %s is in none of %s", errNoUser, acct.name, strings.Join(a.groups, ", "))
	}
	return nil
}

func (a *systemAuth) inGroups(acct *account) (bool, error) {
	if a.method == "pam" {
		return inGroupsNSS(acct, a.groups)
	}
	want := map[string]bool{}
	for _, g := range a.groups {
		want[g] = true
	}
	in := false
	err := scanColon(a.groupFile, func(f []string) bool {
		if len(f) < 4 || !want[f[0]] {
			return false
		}
		if gid, err := strconv.Atoi(f[2]); err == nil && gid == acct.gid {
			in = true
		}
		for _, m := range strings.Split(f[3], ",") {
			if m == acct.name {
				in = true
			}
		}
		return in
	})
	return in, err
}

// checkShadow: the password against the account's hash in the shadow
// file (or passwd, without one), and the account not locked or expired
func (a *systemAuth) checkShadow(acct *account, password string) error {
	hash := acct.passwd
	var sh []string
	if hash == "x" {
		err := scanColon(a.shadowFile, func(f []string) bool {
			if len(f) >= 2 && f[0] == acct.name {
				sh = f
				return true
			}
			return false
		})
		if err != nil {
			return err
		}
		if sh == nil {
			return fmt.Errorf("%s: not in %s", acct.name, a.shadowFile)
		}
		hash = sh[1]
	}
	// no password, a locked or disabled one: no login by password
	if hash == "" || hash[0] == '!' || hash[0] == '*' {
		burn(password)
		return errBadPassword
	}
	ok, err := cryptCompare(hash, password)
	if err != nil {
		return fmt.Errorf("%s: %v", acct.name, err)
	}
	if !ok {
		return errBadPassword
	}
	if err := a.expired(sh); err != nil {
		return fmt.Errorf("%w: %s %v", errBadPassword, acct.name, err)
	}
	return nil
}

// expired: the shadow fields of the account's and its password's ends
// (days since 1970: 2 last change, 4 maximum age, 7 inactivity, 8 expiry)
func (a *systemAuth) expired(sh []string) error {
	if len(sh) < 8 {
		return nil
	}
	day := func(i int) (int, bool) {
		n, err := strconv.Atoi(sh[i])
		return n, err == nil
	}
	today := int(a.now().Unix() / 86400)
	if exp, ok := day(7); ok && exp > 0 && today >= exp {
		return fmt.Errorf("account expired")
	}
	last, okLast := day(2)
	if okLast && last == 0 {
		return fmt.Errorf("must change the password")
	}
	if max, ok := day(4); ok && okLast && max >= 0 && max < 99999 {
		end := last + max
		if inact, ok := day(6); ok && inact >= 0 {
			end += inact
		}
		if today > end {
			return fmt.Errorf("password expired")
		}
	}
	return nil
}

// cryptCompare: a password against a crypt(3) hash of the system:
// yescrypt ($y$), SHA-512 ($6$), SHA-256 ($5$), MD5 ($1$), bcrypt ($2?$)
func cryptCompare(hash, password string) (bool, error) {
	switch {
	case strings.HasPrefix(hash, "$y$"):
		got, err := yescrypt.Hash([]byte(password), []byte(hash))
		if err != nil {
			return false, err
		}
		return subtle.ConstantTimeCompare(got, []byte(hash)) == 1, nil
	case strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"),
		strings.HasPrefix(hash, "$2y$"):
		return checkPassword(hash, password, "bcrypt")
	case crypt.IsHashSupported(hash):
		err := crypt.NewFromHash(hash).Verify(hash, []byte(password))
		if err == crypt.ErrKeyMismatch {
			return false, nil
		}
		return err == nil, err
	}
	scheme := hash
	if i := strings.IndexByte(hash[1:], '$'); hash[0] == '$' && i >= 0 {
		scheme = hash[:i+2]
	} else if len(scheme) > 4 {
		scheme = scheme[:4] + "..."
	}
	return false, fmt.Errorf("password hash %q not supported (yescrypt, SHA-512, SHA-256, MD5, bcrypt; method = pam for others)", scheme)
}

// scanColon: the colon-separated lines of a file to f until it says done
func scanColon(path string, f func([]string) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		if f(strings.Split(line, ":")) {
			return nil
		}
	}
	return sc.Err()
}
