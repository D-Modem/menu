package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The FreePBX sample (freepbx/): it loads as it is, with its users file
// and state directory moved into a test directory, takes logins from the
// users file alone and serves FreePBX's "menu" profile on its socket
func TestFreePBXSample(t *testing.T) {
	dir := t.TempDir()
	conf, err := os.ReadFile("freepbx/modemmenu.conf")
	if err != nil {
		t.Fatal(err)
	}
	users := filepath.Join(dir, "modemmenu.users")
	sample, err := os.ReadFile("freepbx/modemmenu.users")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(users, append(sample, "test:{PLAIN}changeme\n"...), 0600); err != nil {
		t.Fatal(err)
	}
	c := strings.NewReplacer(
		"/etc/modemmenu/modemmenu.users", users,
		"/var/lib/modemmenu", filepath.Join(dir, "state"),
	).Replace(string(conf))
	p := filepath.Join(dir, "modemmenu.conf")
	if err := os.WriteFile(p, []byte(c), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.auth, " ") != "file" || len(s.backends) != 1 {
		t.Errorf("auth %v, want file alone", s.auth)
	}
	if s.server.listen != "/run/modemmenu/modemmenu.sock" || s.server.socketGroup != "asterisk" || !s.server.header {
		t.Errorf("server %+v, want the menu profile's socket, Asterisk's group, the header", *s.server)
	}
	var names []string
	for _, it := range s.items {
		names = append(names, it.name)
	}
	if strings.Join(names, " ") != "ppp callers passwd" {
		t.Errorf("items %v", names)
	}
	ctx := context.Background()
	if err := s.backends[0].authenticate(ctx, "test", "changeme"); err != nil {
		t.Errorf("test's login: %v", err)
	}
	if err := s.backends[0].authenticate(ctx, "test", "wrong"); err == nil {
		t.Error("a wrong password let in")
	}

	// the BBS item, once added to items
	c = strings.Replace(c, "\nitems = ppp callers passwd\n", "\nitems = bbs ppp callers passwd\n", 1)
	if err := os.WriteFile(p, []byte(c), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err = loadSettings(p); err != nil {
		t.Fatal(err)
	} else if len(s.items) != 4 || s.items[0].path != "/run/bbs/menu.sock" || !s.items[0].header {
		t.Errorf("with bbs: %d items, the first %+v", len(s.items), *s.items[0])
	}
}
