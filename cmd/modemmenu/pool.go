// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

// The PPP address pool: each PPP caller gets an address of its own from
// [ppp_pool] range, so that several lines can run PPP at once.  A command
// that names {ppp_remote} (or {ppp_local}, {ppp_dns}) takes an address
// before it starts and gives it back when it ends.  The addresses are lock
// files in state_dir/ppp-pool, held while pppd runs: a modemmenu per call
// and the sessions of modemmenu -serve share them, and an address comes
// back by itself when its modemmenu dies.

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/D-Modem/menu/internal/config"
	"golang.org/x/sys/unix"
)

// pppPoolSettings: [ppp_pool]
type pppPoolSettings struct {
	enabled     bool
	local       netip.Addr   // the server's end of every link
	first, last netip.Addr   // the callers' addresses, both included
	dns         []netip.Addr // offered to the callers (ms-dns), at most 2
}

// pppPoolMax: a range larger than this is a typo, not a pool
const pppPoolMax = 4096

// loadPPPPool: [ppp_pool], checked; no range: no pool
func loadPPPPool(sec *config.Section) (pppPoolSettings, error) {
	var p pppPoolSettings
	r := strings.TrimSpace(sec.Get("range", ""))
	if r == "" {
		return p, nil
	}
	lo, hi, ok := strings.Cut(r, "-")
	if !ok {
		return p, fmt.Errorf("[ppp_pool] range: %q (first-last, e.g. 192.168.77.10-192.168.77.99)", r)
	}
	var err error
	if p.first, err = parseIPv4(strings.TrimSpace(lo)); err != nil {
		return p, fmt.Errorf("[ppp_pool] range: %v", err)
	}
	// the last may be the whole address or its last number alone
	hi = strings.TrimSpace(hi)
	if !strings.Contains(hi, ".") {
		b := p.first.As4()
		hi = fmt.Sprintf("%d.%d.%d.%s", b[0], b[1], b[2], hi)
	}
	if p.last, err = parseIPv4(hi); err != nil {
		return p, fmt.Errorf("[ppp_pool] range: %v", err)
	}
	if p.last.Less(p.first) {
		return p, fmt.Errorf("[ppp_pool] range: %s comes before %s", p.last, p.first)
	}
	if n := addrIndex(p.first, p.last); n >= pppPoolMax {
		return p, fmt.Errorf("[ppp_pool] range: %d addresses (at most %d)", n+1, pppPoolMax)
	}
	l := strings.TrimSpace(sec.Get("local", ""))
	if l == "" {
		return p, fmt.Errorf("[ppp_pool] local: the server's address on the links (e.g. 192.168.77.1)")
	}
	if p.local, err = parseIPv4(l); err != nil {
		return p, fmt.Errorf("[ppp_pool] local: %v", err)
	}
	if !p.local.Less(p.first) && !p.last.Less(p.local) {
		return p, fmt.Errorf("[ppp_pool] local: %s is in the range", p.local)
	}
	for _, d := range config.Fields(sec.Get("dns", l)) {
		a, err := parseIPv4(d)
		if err != nil {
			return p, fmt.Errorf("[ppp_pool] dns: %v", err)
		}
		p.dns = append(p.dns, a)
	}
	if len(p.dns) > 2 {
		return p, fmt.Errorf("[ppp_pool] dns: at most 2 servers (PPP offers a primary and a secondary)")
	}
	p.enabled = true
	return p, nil
}

func parseIPv4(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return netip.Addr{}, fmt.Errorf("%q: not an IPv4 address", s)
	}
	return a, nil
}

// addrIndex: how far b is after a
func addrIndex(a, b netip.Addr) int {
	x, y := a.As4(), b.As4()
	ax := uint32(x[0])<<24 | uint32(x[1])<<16 | uint32(x[2])<<8 | uint32(x[3])
	bx := uint32(y[0])<<24 | uint32(y[1])<<16 | uint32(y[2])<<8 | uint32(y[3])
	return int(bx - ax)
}

// usesPool: whether a command names the pool's placeholders
func usesPool(command string) bool {
	c := strings.ToLower(command)
	for _, k := range []string{"{ppp_remote}", "{ppp_local}", "{ppp_dns}", "{ppp_dns2}"} {
		if strings.Contains(c, k) {
			return true
		}
	}
	return false
}

// pppLease: an address of the pool, until close
type pppLease struct {
	remote netip.Addr
	p      *pppPoolSettings
	f      *os.File
}

// takeAddress: the first free address of the pool, for user (written in
// its lock file, for whoever looks)
func takeAddress(p *pppPoolSettings, stateDir, user string) (*pppLease, error) {
	dir := filepath.Join(stateDir, "ppp-pool")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for a := p.first; ; a = a.Next() {
		f, err := os.OpenFile(filepath.Join(dir, a.String()), os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil {
			f.Truncate(0)
			f.WriteAt([]byte(fmt.Sprintf("%s %d\n", user, os.Getpid())), 0)
			return &pppLease{remote: a, p: p, f: f}, nil
		}
		f.Close()
		if a == p.last {
			break
		}
	}
	return nil, fmt.Errorf("all %d PPP addresses are in use", addrIndex(p.first, p.last)+1)
}

// close: the address is free again
func (l *pppLease) close() {
	if l != nil && l.f != nil {
		l.f.Truncate(0)
		l.f.Close()
		l.f = nil
	}
}

// vars: {ppp_local}, {ppp_remote}, {ppp_dns}, {ppp_dns2} (the first
// again when there is no second, so ms-dns {ppp_dns2} always has one)
func (l *pppLease) vars() map[string]string {
	v := map[string]string{
		"ppp_local":  l.p.local.String(),
		"ppp_remote": l.remote.String(),
	}
	if len(l.p.dns) > 0 {
		v["ppp_dns"], v["ppp_dns2"] = l.p.dns[0].String(), l.p.dns[0].String()
	}
	if len(l.p.dns) > 1 {
		v["ppp_dns2"] = l.p.dns[1].String()
	}
	return v
}

// add: the lease to a command's placeholders and environment (MODEM_PPP_*)
func (l *pppLease) add(v map[string]string, env []string) []string {
	for k, val := range l.vars() {
		v[k] = val
		env = append(env, "MODEM_"+strings.ToUpper(k)+"="+val)
	}
	return env
}

// leaseFor: an address when command names the pool, else nil
func leaseFor(cfg *settings, command, user string) (*pppLease, error) {
	if !usesPool(command) {
		return nil, nil
	}
	return takeAddress(&cfg.pppPool, cfg.limits.stateDir, user)
}

// checkPool: a command that names the pool needs one
func checkPool(s *settings) error {
	if s.pppPool.enabled {
		return nil
	}
	for _, it := range s.items {
		if it.mode == "exec" && usesPool(it.command) {
			return fmt.Errorf("[item %s] command: {ppp_...} needs [ppp_pool] range", it.name)
		}
	}
	if s.autoppp.enabled && usesPool(s.autoppp.command) {
		return fmt.Errorf("[autoppp] command: {ppp_...} needs [ppp_pool] range")
	}
	return nil
}
