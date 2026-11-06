// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/auth"
	"github.com/D-Modem/menu/internal/config"
)

// Who may use what: groups of users, the access rules of the items, the
// guest account, signup, caller ID rules and time limits.

// itemAccess: an item's rules; all that are set must pass
type itemAccess struct {
	users     []string // these users ...
	groups    []string // ... or the members of these groups (either set: only them)
	minBaud   int      // the connect rate at least this (an unknown rate does not pass)
	callerIDs []string // the caller ID matching one of these patterns
	guest     bool     // guests see it (they see nothing else)
}

// loadItemAccess: an item's access rules (users, groups, callerids,
// min_baud, guest), its caller ID patterns checked
func loadItemAccess(name string, sec *config.Section) (itemAccess, error) {
	var a itemAccess
	var err error
	a.users = config.Fields(sec.Get("users", ""))
	a.groups = config.Fields(sec.Get("groups", ""))
	a.callerIDs = config.Fields(sec.Get("callerids", ""))
	if a.minBaud, err = sec.Int("min_baud", 0); err != nil {
		return a, err
	}
	if a.guest, err = sec.Bool("guest", false); err != nil {
		return a, err
	}
	for _, p := range a.callerIDs {
		if _, err := path.Match(p, ""); err != nil {
			return a, fmt.Errorf("[item %s] callerids: %q: %v", name, p, err)
		}
	}
	return a, nil
}

// guestSettings: [guest], a login without a password
type guestSettings struct {
	enabled   bool
	name      string
	timeLimit time.Duration
}

// signupSettings: [signup], new accounts in the users file
type signupSettings struct {
	enabled     bool
	name        string // typed at the login prompt
	file        string
	pending     bool // new accounts locked until the sysop unlocks them
	minPassword int
	message     string
}

// numberRule: a [number PATTERN] section
type numberRule struct {
	pattern string
	deny    bool
	user    string // logged in as, without a password
	item    string // started at once after the login
}

// callerIDSettings: [callerid] and the [number ...] sections
type callerIDSettings struct {
	allow       []string
	deny        []string
	anonymous   bool // a call without caller ID is taken
	denyMessage string
	numbers     []numberRule
}

// limitSettings: time limits, [general]
type limitSettings struct {
	timeLimit  time.Duration // a call, from the login
	dailyLimit time.Duration // a user's calls in a day
	stateDir   string        // where the daily use is kept
	// written to the line these long before the deadline, in a selection
	// with time_warning; the longest first
	warnings       []time.Duration
	warningMessage string
}

// loadAccess: the time limits and warnings of [general], [groups],
// [guest], [signup] and [callerid] and its [number ...] sections
func loadAccess(c *config.File, s *settings) error {
	var err error
	g := c.Section("general")
	if s.limits.timeLimit, err = g.Duration("time_limit", 0); err != nil {
		return err
	}
	if s.limits.dailyLimit, err = g.Duration("daily_limit", 0); err != nil {
		return err
	}
	s.limits.stateDir = g.Get("state_dir", "/var/lib/modemmenu")
	if s.limits.warnings, err = g.Durations("time_warning", "2m"); err != nil {
		return err
	}
	s.limits.warningMessage = config.Unescape(g.Get("time_warning_message",
		`\r\n\r\n*** {time_left} minute(s) left in this call ***\r\n`))
	if s.limits.dailyLimit > 0 && !strings.HasPrefix(s.limits.stateDir, "/") {
		return fmt.Errorf("[general] state_dir: %q (an absolute path)", s.limits.stateDir)
	}

	// [groups]: name = its members
	s.groups = map[string][]string{}
	gs := c.Section("groups")
	for _, k := range gs.Keys {
		s.groups[k] = config.Fields(gs.Get(k, ""))
	}

	gu := c.Section("guest")
	if s.guest.enabled, err = gu.Bool("enabled", false); err != nil {
		return err
	}
	s.guest.name = gu.Get("name", "guest")
	if s.guest.timeLimit, err = gu.Duration("time_limit", 15*time.Minute); err != nil {
		return err
	}

	su := c.Section("signup")
	if s.signup.enabled, err = su.Bool("enabled", false); err != nil {
		return err
	}
	s.signup.name = su.Get("name", "new")
	s.signup.file = su.Get("file", c.Section("file").Get("path", "/etc/asterisk/modemmenu.users"))
	if s.signup.pending, err = su.Bool("pending", true); err != nil {
		return err
	}
	if s.signup.minPassword, err = su.Int("min_password", 8); err != nil {
		return err
	}
	s.signup.message = config.Unescape(su.Get("message", `\r\nNew account.  Choose a user name and a password.\r\n`))
	if s.signup.enabled && !contains(s.auth, "file") {
		return fmt.Errorf("[signup] enabled: new accounts go in the users file, and [general] auth has no file")
	}
	if s.guest.enabled && s.signup.enabled && strings.EqualFold(s.guest.name, s.signup.name) {
		return fmt.Errorf("[guest] name and [signup] name are both %q", s.guest.name)
	}

	cs := c.Section("callerid")
	s.callerID.allow = config.Fields(cs.Get("allow", ""))
	s.callerID.deny = config.Fields(cs.Get("deny", ""))
	if s.callerID.anonymous, err = cs.Bool("anonymous", true); err != nil {
		return err
	}
	s.callerID.denyMessage = config.Unescape(cs.Get("deny_message", `\r\nThis line does not take your call.\r\n`))
	for _, p := range append(append([]string{}, s.callerID.allow...), s.callerID.deny...) {
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("[callerid] %q: %v", p, err)
		}
	}
	for _, sec := range c.Sections {
		p, ok := strings.CutPrefix(sec.Name, "number ")
		if !ok {
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("[%s]: %v", sec.Name, err)
		}
		r := numberRule{pattern: p, user: sec.Get("user", ""), item: sec.Get("item", "")}
		if r.deny, err = sec.Bool("deny", false); err != nil {
			return err
		}
		s.callerID.numbers = append(s.callerID.numbers, r)
	}
	return nil
}

// checkAccess after the items are loaded: what refers to them exists
func checkAccess(s *settings) error {
	for _, r := range s.callerID.numbers {
		if r.item != "" && s.selects(strings.ToUpper(r.item)) == nil {
			return fmt.Errorf("[number %s] item: no item %q", r.pattern, r.item)
		}
		if r.user != "" && !s.userPattern.MatchString(r.user) {
			return fmt.Errorf("[number %s] user: %q does not match user_pattern", r.pattern, r.user)
		}
	}
	for _, it := range s.items {
		for _, g := range it.access.groups {
			if _, ok := s.groups[g]; !ok && !contains(s.auth, "system") && !contains(s.auth, "pam") && !s.ldapGroups() {
				return fmt.Errorf("[item %s] groups: no %s in [groups]", it.name, g)
			}
		}
	}
	return nil
}

// matchNumber: a caller ID against patterns (* and ? as in file names);
// "anonymous" matches a call without one
func matchNumber(patterns []string, number string) bool {
	for _, p := range patterns {
		if number == "" {
			if p == "anonymous" {
				return true
			}
			continue
		}
		if ok, _ := path.Match(p, number); ok {
			return true
		}
	}
	return false
}

// callerAllowed: whether the line takes the call, by [callerid] and the
// [number ...] rules
func (s *settings) callerAllowed(number string) bool {
	cid := s.callerID
	if number == "" && !cid.anonymous {
		return false
	}
	if r := s.numberRule(number); r != nil && r.deny {
		return false
	}
	if matchNumber(cid.deny, number) {
		return false
	}
	if len(cid.allow) > 0 && number != "" && !matchNumber(cid.allow, number) {
		return false
	}
	return true
}

// numberRule: the first [number ...] whose pattern the caller ID matches
func (s *settings) numberRule(number string) *numberRule {
	if number == "" {
		return nil
	}
	for i, r := range s.callerID.numbers {
		if ok, _ := path.Match(r.pattern, number); ok {
			return &s.callerID.numbers[i]
		}
	}
	return nil
}

// inGroup: the session's user is a member of group, by [groups]; for a
// system account (auth = system) by the system's groups; for an LDAP one
// by the directory's (with [ldap] group_base).  The backends' answers are
// kept for the call.
func (s *settings) inGroup(sess *session, group string) bool {
	if contains(s.groups[group], sess.User) {
		return true
	}
	via := strings.TrimPrefix(sess.Auth, "upstream:")
	if via != "system" && via != "ldap" {
		return false
	}
	if in, ok := sess.groupCache[group]; ok {
		return in
	}
	var in bool
	var err error
	if via == "system" {
		var acct *auth.Account
		if acct, err = auth.LookupNSS(sess.User); err == nil {
			in, err = auth.InGroupsNSS(acct, []string{group})
		}
	} else {
		for _, b := range s.backends {
			if la, ok := b.(*auth.LDAP); ok {
				in, err = la.MemberOf(sess.User, sess.Password, group)
			}
		}
	}
	if err != nil && !errors.Is(err, auth.ErrNoUser) {
		logf("groups: %s in %s: %v", sess.User, group, err)
	}
	if sess.groupCache == nil {
		sess.groupCache = map[string]bool{}
	}
	sess.groupCache[group] = in && err == nil
	return sess.groupCache[group]
}

// allows: whether the session may see and use the item
func (s *settings) allows(it *menuItem, sess *session) bool {
	a := it.access
	if sess.Auth == "guest" {
		if !a.guest {
			return false
		}
	} else if len(a.users) > 0 || len(a.groups) > 0 {
		ok := contains(a.users, sess.User)
		for _, g := range a.groups {
			ok = ok || s.inGroup(sess, g)
		}
		if !ok {
			return false
		}
	}
	if a.minBaud > 0 {
		baud, err := strconv.Atoi(sess.Baud)
		if err != nil || baud < a.minBaud {
			return false
		}
	}
	if len(a.callerIDs) > 0 && !matchNumber(a.callerIDs, sess.CallerID) {
		return false
	}
	return true
}

// visible: the items the session may use, in menu order
func (s *settings) visible(sess *session) []*menuItem {
	var items []*menuItem
	for _, it := range s.items {
		if s.allows(it, sess) {
			items = append(items, it)
		}
	}
	return items
}

// ldapGroups: whether the LDAP backend has groups ([ldap] group_base)
func (s *settings) ldapGroups() bool {
	for _, b := range s.backends {
		if la, ok := b.(*auth.LDAP); ok && la.HasGroups() {
			return true
		}
	}
	return false
}
