// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/D-Modem/menu/internal/config"
)

// poolConf: a configuration with [ppp_pool] pool and an item command
func poolConf(t *testing.T, pool, command string) (*settings, error) {
	t.Helper()
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	p := filepath.Join(dir, "c.conf")
	os.WriteFile(p, []byte("[general]\nlog = none\nauth = file\nstate_dir = "+dir+"\n[file]\npath = "+users+
		"\n[ppp_pool]\n"+pool+"\n[item ppp]\nkey = P\nmode = exec\ncommand = "+command+"\n"), 0o600)
	return loadSettings(p)
}

// the addresses: each taken once, given back on close, none past the last
func TestPPPPool(t *testing.T) {
	s, err := poolConf(t, "local = 10.77.0.1\nrange = 10.77.0.10-12\ndns = 10.77.0.1 9.9.9.9",
		"pppd {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2}")
	if err != nil {
		t.Fatal(err)
	}
	var leases []*pppLease
	for _, want := range []string{"10.77.0.10", "10.77.0.11", "10.77.0.12"} {
		l, err := leaseFor(s, s.items[0].command, "alice")
		if err != nil {
			t.Fatal(err)
		}
		if l.remote.String() != want {
			t.Errorf("got %s, want %s", l.remote, want)
		}
		leases = append(leases, l)
	}
	if _, err := leaseFor(s, s.items[0].command, "bob"); err == nil || !strings.Contains(err.Error(), "all 3") {
		t.Errorf("a full pool: %v", err)
	}
	// who has it, for whoever looks
	b, _ := os.ReadFile(filepath.Join(s.limits.stateDir, "ppp-pool", "10.77.0.11"))
	if !strings.HasPrefix(string(b), "alice ") {
		t.Errorf("lock file: %q", b)
	}
	leases[1].close()
	l, err := leaseFor(s, s.items[0].command, "bob")
	if err != nil || l.remote.String() != "10.77.0.11" {
		t.Fatalf("after a close: %v %v", l, err)
	}
	v := map[string]string{}
	env := l.add(v, nil)
	got := config.Expand(s.items[0].command, v, nil)
	if got != "pppd 10.77.0.1:10.77.0.11 ms-dns 10.77.0.1 ms-dns 9.9.9.9" {
		t.Errorf("command: %s", got)
	}
	if !strings.Contains(strings.Join(env, " "), "MODEM_PPP_REMOTE=10.77.0.11") {
		t.Errorf("env: %v", env)
	}
	// a command without the placeholders takes nothing
	if l, err := leaseFor(s, "pppd 1.2.3.4:5.6.7.8", "carol"); l != nil || err != nil {
		t.Errorf("no placeholders: %v %v", l, err)
	}
}

// one DNS server: {ppp_dns2} repeats it, so ms-dns always has one
func TestPPPPoolOneDNS(t *testing.T) {
	s, err := poolConf(t, "local = 10.77.0.1\nrange = 10.77.0.10-10.77.0.20", "pppd {ppp_remote}")
	if err != nil {
		t.Fatal(err)
	}
	l, err := takeAddress(&s.pppPool, s.limits.stateDir, "x")
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if v := l.vars(); v["ppp_dns"] != "10.77.0.1" || v["ppp_dns2"] != "10.77.0.1" {
		t.Errorf("dns defaults to local: %v", v)
	}
}

func TestPPPPoolSettings(t *testing.T) {
	for pool, want := range map[string]string{
		"range = 10.77.0.10":                                                      "first-last",
		"range = 10.77.0.20-10":                                                   "comes before",
		"range = 10.77.0.10-20":                                                   "local:",
		"local = 10.77.0.15\nrange = 10.77.0.10-20":                               "in the range",
		"local = 10.77.0.1\nrange = 10.77.0.10-10.78.0.10":                        "at most",
		"local = 10.77.0.1\nrange = fe80::1-2":                                    "not an IPv4",
		"local = 10.77.0.1\nrange = 10.77.0.10-20\ndns = a":                       "dns",
		"local = 10.77.0.1\nrange = 10.77.0.10-20\ndns = 1.1.1.1 8.8.8.8 9.9.9.9": "at most 2",
	} {
		if _, err := poolConf(t, pool, "pppd {ppp_remote}"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", pool, err, want)
		}
	}
	// a command naming the pool, with no pool
	if _, err := poolConf(t, "", "pppd {ppp_remote}"); err == nil || !strings.Contains(err.Error(), "needs [ppp_pool]") {
		t.Errorf("no pool: %v", err)
	}
}
