package main

import (
	"context"
	"errors"
	"os"
	"testing"
)

// MODEMMENU_TEST_LDAP=ldap://host:port of a server with
// uid=lduser,ou=people,dc=example,dc=com, password ldpass
func TestLDAP(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
	for _, vals := range []map[string]string{
		{"url": url, "search_base": "ou=people,dc=example,dc=com"},
		{"url": url, "bind_dn": "uid={user},ou=people,dc=example,dc=com"},
	} {
		a, err := newLDAPAuth(&section{name: "ldap", vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := a.authenticate(ctx, "lduser", "ldpass"); err != nil {
			t.Errorf("%v: right password: %v", vals, err)
		}
		if err := a.authenticate(ctx, "lduser", "wrong"); err == nil {
			t.Errorf("%v: wrong password accepted", vals)
		}
		if err := a.authenticate(ctx, "lduser", ""); err == nil {
			t.Errorf("%v: empty password accepted", vals)
		}
		if err := a.authenticate(ctx, "nobody*)(uid=*", "ldpass"); !errors.Is(err, errNoUser) {
			t.Errorf("%v: unknown user: %v", vals, err)
		}
	}
}
