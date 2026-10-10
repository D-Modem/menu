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
}

func newLDAPAuth(s *section) (authenticator, error) {
	var err error
	a := &ldapAuth{
		url:        s.get("url", "ldap://127.0.0.1:389"),
		bindDN:     s.get("bind_dn", ""),
		base:       s.get("search_base", ""),
		filter:     s.get("search_filter", "(uid={user})"),
		searchDN:   s.get("search_bind_dn", ""),
		searchPass: s.get("search_bind_password", ""),
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

func (a *ldapAuth) name() string { return "ldap" }

func (a *ldapAuth) authenticate(ctx context.Context, user, password string) error {
	// an empty password would be an unauthenticated bind, which servers
	// accept for any DN
	if password == "" {
		return errBadPassword
	}
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
		return err
	}
	defer conn.Close()
	conn.SetTimeout(a.timeout)
	if a.startTLS {
		if err := conn.StartTLS(tc); err != nil {
			return fmt.Errorf("starttls: %v", err)
		}
	}

	dn := ""
	if a.bindDN != "" {
		dn = expand(a.bindDN, map[string]string{"user": user}, ldap.EscapeDN)
	} else {
		if a.searchDN != "" {
			if err := conn.Bind(a.searchDN, a.searchPass); err != nil {
				return fmt.Errorf("bind %s: %v", a.searchDN, err)
			}
		}
		filter := expand(a.filter, map[string]string{"user": user}, ldap.EscapeFilter)
		res, err := conn.Search(ldap.NewSearchRequest(a.base, ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases, 2, int(a.timeout/time.Second), false,
			filter, []string{"dn"}, nil))
		if err != nil {
			return fmt.Errorf("search %s: %v", filter, err)
		}
		switch len(res.Entries) {
		case 0:
			return errNoUser
		case 1:
			dn = res.Entries[0].DN
		default:
			return fmt.Errorf("search %s: more than one entry", filter)
		}
	}
	err = conn.Bind(dn, password)
	var le *ldap.Error
	if errors.As(err, &le) && le.ResultCode == ldap.LDAPResultInvalidCredentials {
		if a.bindDN != "" {
			// a direct bind cannot tell an unknown DN from a wrong
			// password: let the other backends try
			return errNoUser
		}
		return errBadPassword
	}
	return err
}
