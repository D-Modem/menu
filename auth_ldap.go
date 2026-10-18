// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/go-ldap/ldap/v3"
)

// ldapAuth: a bind as the user, to bind_dn with {user} in it, or to the
// DN that search_filter finds under search_base (searched as
// search_bind_dn, anonymously if none)
type ldapAuth struct {
	url        string
	startTLS   bool
	tls        *tls.Config
	timeout    time.Duration
	bindDN     string
	base       string
	filter     string
	searchDN   string
	searchPass string
	// groups for the items' groups = (group_base set): a group is an
	// entry under group_base that group_filter finds
	groupBase   string
	groupFilter string
	// how a password is changed: exop (Password Modify) or ad
	// (Active Directory's unicodePwd)
	passwordChange string
}

// newLDAPAuth: the [ldap] backend from its section, checked
func newLDAPAuth(s *section) (authenticator, error) {
	var err error
	a := &ldapAuth{
		url:        s.get("url", "ldap://127.0.0.1:389"),
		bindDN:     s.get("bind_dn", ""),
		base:       s.get("search_base", ""),
		filter:     s.get("search_filter", "(uid={user})"),
		searchDN:   s.get("search_bind_dn", ""),
		searchPass: s.get("search_bind_password", ""),
		groupBase:  s.get("group_base", ""),
		groupFilter: s.get("group_filter",
			"(&(cn={group})(|(member={dn})(uniqueMember={dn})(memberUid={user})))"),
		passwordChange: strings.ToLower(s.get("password_change", "exop")),
	}
	if a.passwordChange != "exop" && a.passwordChange != "ad" {
		return nil, fmt.Errorf("[ldap] password_change: %q (exop or ad)", a.passwordChange)
	}
	if a.startTLS, err = s.bool("starttls", false); err != nil {
		return nil, err
	}
	if a.timeout, err = s.duration("timeout", 10*time.Second); err != nil {
		return nil, err
	}
	skip, err := s.bool("insecure_skip_verify", false)
	if err != nil {
		return nil, err
	}
	a.tls = &tls.Config{InsecureSkipVerify: skip}
	if ca := s.get("ca_file", ""); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("[ldap] ca_file: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("[ldap] ca_file: no certificate in %s", ca)
		}
		a.tls.RootCAs = pool
	}
	if a.bindDN == "" && a.base == "" {
		return nil, fmt.Errorf("[ldap] needs bind_dn or search_base")
	}
	if a.bindDN != "" && !strings.Contains(a.bindDN, "{user}") {
		return nil, fmt.Errorf("[ldap] bind_dn: no {user} in %q", a.bindDN)
	}
	return a, nil
}

// name: the backend's name, for {auth} and the log
func (a *ldapAuth) name() string { return "ldap" }

// authenticate: a bind as the user (found by search, or by bind_dn)
func (a *ldapAuth) authenticate(ctx context.Context, user, password string) error {
	conn, _, err := a.bindUser(user, password)
	if conn != nil {
		conn.Close()
	}
	return err
}

// bindUser: a connection bound as the user, and the user's DN
func (a *ldapAuth) bindUser(user, password string) (*ldap.Conn, string, error) {
	// an empty password would be an unauthenticated bind, which servers
	// accept for any DN
	if password == "" {
		return nil, "", errBadPassword
	}
	conn, err := a.dial()
	if err != nil {
		return nil, "", err
	}
	dn, err := a.userDN(conn, user)
	if err != nil {
		conn.Close()
		return nil, "", err
	}
	err = conn.Bind(dn, password)
	var le *ldap.Error
	if errors.As(err, &le) && le.ResultCode == ldap.LDAPResultInvalidCredentials {
		conn.Close()
		if a.bindDN != "" {
			// a direct bind cannot tell an unknown DN from a wrong
			// password: let the other backends try
			return nil, "", errNoUser
		}
		return nil, "", errBadPassword
	}
	if err != nil {
		conn.Close()
		return nil, "", err
	}
	return conn, dn, nil
}

// dial the server, with StartTLS if configured
func (a *ldapAuth) dial() (*ldap.Conn, error) {
	d := &net.Dialer{Timeout: a.timeout}
	host := a.url
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	host, _, _ = strings.Cut(host, "/")
	tc := a.tls.Clone()
	if tc.ServerName == "" {
		tc.ServerName, _, _ = strings.Cut(host, ":")
	}
	conn, err := ldap.DialURL(a.url, ldap.DialWithDialer(d), ldap.DialWithTLSConfig(tc))
	if err != nil {
		return nil, err
	}
	conn.SetTimeout(a.timeout)
	if a.startTLS {
		if err := conn.StartTLS(tc); err != nil {
			conn.Close()
			return nil, fmt.Errorf("starttls: %v", err)
		}
	}
	return conn, nil
}

// userDN: the user's DN, from bind_dn or by the search
func (a *ldapAuth) userDN(conn *ldap.Conn, user string) (string, error) {
	if a.bindDN != "" {
		return expand(a.bindDN, map[string]string{"user": user}, ldap.EscapeDN), nil
	}
	if a.searchDN != "" {
		if err := conn.Bind(a.searchDN, a.searchPass); err != nil {
			return "", fmt.Errorf("bind %s: %v", a.searchDN, err)
		}
	}
	filter := expand(a.filter, map[string]string{"user": user}, ldap.EscapeFilter)
	res, err := conn.Search(ldap.NewSearchRequest(a.base, ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 2, int(a.timeout/time.Second), false,
		filter, []string{"dn"}, nil))
	if err != nil {
		return "", fmt.Errorf("search %s: %v", filter, err)
	}
	switch len(res.Entries) {
	case 0:
		return "", errNoUser
	case 1:
		return res.Entries[0].DN, nil
	}
	return "", fmt.Errorf("search %s: more than one entry", filter)
}

// changePassword bound as the user, so that the server applies its own
// password policy: by the Password Modify extended operation (RFC 3062),
// or for Active Directory (password_change = ad, which does not have it)
// by replacing unicodePwd, the old value deleted and the new one added
// (which AD takes only over TLS: ldaps:// or starttls)
func (a *ldapAuth) changePassword(_ context.Context, _ *term, user, old, new string) error {
	conn, dn, err := a.bindUser(user, old)
	if err != nil {
		return err
	}
	defer conn.Close()
	if a.passwordChange == "ad" {
		err = conn.Modify(adPasswordChange(dn, old, new))
	} else {
		_, err = conn.PasswordModify(ldap.NewPasswordModifyRequest(dn, old, new))
	}
	if err != nil {
		return fmt.Errorf("%w: %v", errRefused, err)
	}
	return nil
}

// adPasswordChange: AD's change of a user's own password, unicodePwd's
// old value deleted and the new one added, each in "quotes" as UTF-16LE
func adPasswordChange(dn, old, new string) *ldap.ModifyRequest {
	m := ldap.NewModifyRequest(dn, nil)
	m.Delete("unicodePwd", []string{adPassword(old)})
	m.Add("unicodePwd", []string{adPassword(new)})
	return m
}

// adPassword: a password as AD's unicodePwd takes it, in double quotes
// and UTF-16LE
func adPassword(pw string) string {
	u := utf16.Encode([]rune(`"` + pw + `"`))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i] = byte(c)
		b[2*i+1] = byte(c >> 8)
	}
	return string(b)
}

// memberOf: whether user is in group, an entry under group_base that
// group_filter ({group}, the user's {dn} and {user}) finds; searched as
// search_bind_dn, else as the user (with their password), else
// anonymously.  errNoUser without group_base: LDAP has no groups here.
func (a *ldapAuth) memberOf(user, password, group string) (bool, error) {
	if a.groupBase == "" {
		return false, errNoUser
	}
	var conn *ldap.Conn
	var dn string
	var err error
	if a.searchDN == "" && password != "" {
		conn, dn, err = a.bindUser(user, password)
	} else {
		if conn, err = a.dial(); err == nil {
			// userDN binds as search_bind_dn when there is one
			if dn, err = a.userDN(conn, user); err != nil {
				conn.Close()
			}
		}
	}
	if err != nil {
		return false, err
	}
	defer conn.Close()
	filter := expand(a.groupFilter, map[string]string{"group": group, "dn": dn, "user": user}, ldap.EscapeFilter)
	res, err := conn.Search(ldap.NewSearchRequest(a.groupBase, ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 1, int(a.timeout/time.Second), false,
		filter, []string{"dn"}, nil))
	if err != nil {
		return false, fmt.Errorf("group search %s: %v", filter, err)
	}
	return len(res.Entries) > 0, nil
}
