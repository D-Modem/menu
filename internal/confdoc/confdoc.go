// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package confdoc reads what samples/modemmenu.conf.sample says of each
// section and key in its comments: the help, the sample's value and the
// other values it shows.  tools/confref makes the configuration reference
// of it, and modemmenu -setup the help of its screens.
package confdoc

import (
	"regexp"
	"strings"
)

// Kind is a kind of section: [general], [item NAME] ...
type Kind struct {
	Name  string
	Named bool
	Help  string
	Free  string // what its keys are, when they are the sysop's own names
	Keys  []*Key
	ByKey map[string]*Key
}

// Key is one key of a kind of section.
type Key struct {
	Name     string
	Help     string
	Value    string   // the sample's (or its first commented example's)
	Examples []string // the other values the sample shows
}

var (
	headerRE  = regexp.MustCompile(`^;?\[([a-z_]+)(?:\s+(\S+))?\]$`)
	keyRE     = regexp.MustCompile(`^(;?)([a-z][a-z0-9_]*)\s*=\s*(.*)$`)
	docKeyRE  = regexp.MustCompile(`^;\s{2,4}([a-z][a-z0-9_]*) = (\S+)\s*(.*)$`)
	docMoreRE = regexp.MustCompile(`^;\s{5,}(.*)$`)
	// a value of a described key and what it does ("passwd   built in")
	docValueRE = regexp.MustCompile(`^([a-z0-9_]+)\s{2,}(\S.*)$`)
)

// Unquote is a sample value as the sysop writes it: without its quotes.
func Unquote(v string) string {
	if i := strings.Index(v, " ; "); i >= 0 && !strings.HasPrefix(v, `"`) {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// kinds' help where the sample has none before them, or has the first
// example's
var kindHelp = map[string]string{
	"general": "The login: the log, the backends, what the caller sees, the time limits.",
	"menu":    "The menu: its title, prompt, items and the words that hang up.",
	"item":    "A menu item: a program on the line, a server to connect the caller to, or a built-in (see the item keys below).",
}

// kinds whose keys are the sysop's own names, and what such a key is
var freeKinds = map[string]string{
	"groups": "a group: its name, then its members (`sysops = alice bob`)",
}

func Parse(sample string) []*Kind {
	var kinds []*Kind
	byName := map[string]*Kind{}
	get := func(name string) *Kind {
		if k := byName[name]; k != nil {
			return k
		}
		k := &Kind{Name: name, ByKey: map[string]*Key{}}
		byName[name] = k
		kinds = append(kinds, k)
		return k
	}
	var cur *Kind
	var prose []string
	keyOf := func(k *Kind, name string) *Key {
		if x := k.ByKey[name]; x != nil {
			return x
		}
		x := &Key{Name: name}
		k.ByKey[name] = x
		k.Keys = append(k.Keys, x)
		return x
	}
	var doc *Key       // the key an indented doc line describes
	var pending []*Key // described keys not yet given to a kind
	adopt := func(k *Kind) {
		for _, d := range pending {
			x := keyOf(k, d.Name)
			if x.Value == "" {
				x.Value = d.Value
			}
			if x.Help == "" {
				x.Help = d.Help
			}
		}
		pending = nil
	}
	docBlock := false
	inHeader := true  // the file's own comment before the first section
	afterKey := false // a comment now starts the help of the keys after it
	for _, raw := range strings.Split(sample, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			prose, doc, docBlock, afterKey = nil, nil, false, false
			continue
		}
		if m := headerRE.FindStringSubmatch(line); m != nil {
			inHeader = false
			cur = get(m[1])
			adopt(cur)
			if m[2] != "" {
				cur.Named = true
			}
			if cur.Help == "" && len(prose) > 0 {
				cur.Help = strings.Join(prose, " ")
			}
			prose, doc, afterKey = nil, nil, false
			continue
		}
		if inHeader {
			continue
		}
		// an indented description of keys (";   key = B   its key"): of
		// the kind of section whose keys or header come next
		if m := docKeyRE.FindStringSubmatch(line); m != nil && (docBlock || len(prose) > 0) {
			docBlock = true
			doc = &Key{Name: m[1], Value: Unquote(m[2]), Help: m[3]}
			pending = append(pending, doc)
			continue
		}
		if m := docMoreRE.FindStringSubmatch(line); m != nil && doc != nil {
			more := m[1]
			if v := docValueRE.FindStringSubmatch(more); v != nil {
				more = "; `" + v[1] + "`: " + v[2]
			}
			sep := " "
			if strings.HasPrefix(more, ";") {
				sep = ""
			}
			doc.Help = strings.TrimSpace(doc.Help + sep + more)
			continue
		}
		if m := keyRE.FindStringSubmatch(line); m != nil && cur != nil {
			adopt(cur)
			x := keyOf(cur, m[2])
			v := Unquote(m[3])
			if x.Help == "" && len(prose) > 0 {
				x.Help = strings.Join(prose, " ")
			}
			if x.Value == "" && m[1] == "" {
				x.Value = v
			} else if v != "" && v != x.Value {
				seen := false
				for _, e := range x.Examples {
					seen = seen || e == v
				}
				if !seen {
					x.Examples = append(x.Examples, v)
				}
			}
			afterKey = true
			continue
		}
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			t := strings.TrimSpace(strings.TrimLeft(line, ";#"))
			if doc != nil || docBlock || afterKey {
				// prose after a block of key descriptions starts afresh
				prose, doc, docBlock, afterKey = nil, nil, false, false
			}
			prose = append(prose, t)
		}
	}
	if cur != nil {
		adopt(cur)
	}
	for _, k := range kinds {
		if h, ok := kindHelp[k.Name]; ok {
			k.Help = h
		}
		if f, ok := freeKinds[k.Name]; ok {
			k.Free, k.Keys = f, nil
		}
	}
	// a key with no value of its own: its first example is it
	for _, k := range kinds {
		for _, x := range k.Keys {
			if x.Value == "" && len(x.Examples) > 0 {
				x.Value, x.Examples = x.Examples[0], x.Examples[1:]
			}
		}
	}
	return kinds
}
