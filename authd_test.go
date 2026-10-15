// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func authSettings(t *testing.T, extra string) (*settings, string) {
	t.Helper()
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	// pending: a signup the sysop has not unlocked
	os.WriteFile(users, []byte("alice:{PLAIN}secret\npending:!{PLAIN}secret\n"), 0o600)
	sock := filepath.Join(dir, "auth.sock")
	p := filepath.Join(dir, "c.conf")
	os.WriteFile(p, []byte(fmt.Sprintf("[general]\nauth = file\nfail_delay = 0\n[file]\npath = %s\n[authd]\nlisten = %s\nsocket_mode = 0600\n%s"+
		"[item x]\nkey = X\ncommand = true\n", users, sock, extra)), 0o600)
	s, err := loadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	return s, sock
}

func ask(t *testing.T, sock, body string) authReply {
	t.Helper()
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte(body + "\n"))
	var r authReply
	if err := json.NewDecoder(c).Decode(&r); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	return r
}

// TestAuthd: the auth socket answers by the backends, user_pattern kept.
func TestAuthd(t *testing.T) {
	s, sock := authSettings(t, "")
	ln, err := listenAuth(s.authd)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveAuth(s, ln)
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket %v %v", fi, err)
	}
	for body, want := range map[string]authReply{
		`{"user":"alice","password":"secret"}`:   {OK: true, Auth: "file"},
		`{"user":"alice","password":"wrong"}`:    {Error: "denied"},
		`{"user":"bob","password":"secret"}`:     {Error: "denied"},
		`{"user":"pending","password":"secret"}`: {Error: "denied"},
		`{"user":"../etc","password":"secret"}`:  {Error: "denied"},
		`{"user":"","password":"secret"}`:        {Error: "bad request"},
		`not json`:                               {Error: "bad request"},
		`{"user":"alice","password":"secret"`:    {Error: "bad request"},
	} {
		if got := ask(t, sock, body); got != want {
			t.Errorf("%s: %+v, want %+v", body, got, want)
		}
	}
}

// TestAuthdAllowUsers: a peer not among allow_users is refused before it
// is heard.
func TestAuthdAllowUsers(t *testing.T) {
	if os.Getuid() == 65534 {
		t.Skip("running as nobody")
	}
	s, sock := authSettings(t, "allow_users = nobody\n")
	ln, err := listenAuth(s.authd)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveAuth(s, ln)
	if got := ask(t, sock, `{"user":"alice","password":"secret"}`); got.OK || got.Error != "refused" {
		t.Errorf("%+v", got)
	}
}

func TestAuthdSettings(t *testing.T) {
	for extra, want := range map[string]string{
		"listen = tcp:127.0.0.1:9\n":   "a unix socket only",
		"socket_mode = 0666\n":         "lets anyone ask",
		"allow_users = no-such-user\n": "allow_users",
		"max_clients = 0\n":            "max_clients",
	} {
		dir := t.TempDir()
		users := filepath.Join(dir, "users")
		os.WriteFile(users, nil, 0o600)
		p := filepath.Join(dir, "c.conf")
		os.WriteFile(p, []byte("[general]\nauth = file\n[file]\npath = "+users+"\n[authd]\nlisten = /run/x.sock\n"+extra+
			"[item x]\nkey = X\ncommand = true\n"), 0o600)
		if _, err := loadSettings(p); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", extra, err)
		}
	}
}
