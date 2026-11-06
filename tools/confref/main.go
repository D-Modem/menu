// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Command confref writes docs/reference.md, the configuration reference:
// every section and key of modemmenu.conf, with the help, the sample's
// value and the other values the sample shows, all read from the comments
// of modemmenu.conf.sample.  From the repository's root:
//
//	go run ./tools/confref
//
// TestSampleHasEveryKey (config_test.go) keeps the sample complete: a key
// the code reads that the sample doesn't show fails it.
package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// kind is a kind of section: [general], [item NAME] ...
type kind struct {
	name  string
	named bool
	help  string
	free  string // what its keys are, when they are the sysop's own names
	keys  []*key
	byKey map[string]*key
}

// key is one key of a kind of section.
type key struct {
	name     string
	help     string
	value    string   // the sample's (or its first commented example's)
	examples []string // the other values the sample shows
}

var (
	headerRE  = regexp.MustCompile(`^;?\[([a-z]+)(?:\s+(\S+))?\]$`)
	keyRE     = regexp.MustCompile(`^(;?)([a-z][a-z0-9_]*)\s*=\s*(.*)$`)
	docKeyRE  = regexp.MustCompile(`^;\s{2,4}([a-z][a-z0-9_]*) = (\S+)\s*(.*)$`)
	docMoreRE = regexp.MustCompile(`^;\s{5,}(.*)$`)
	// a value of a described key and what it does ("passwd   built in")
	docValueRE = regexp.MustCompile(`^([a-z0-9_]+)\s{2,}(\S.*)$`)
)

// unquote is a sample value as the sysop writes it: without its quotes.
func unquote(v string) string {
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

func parse(sample string) []*kind {
	var kinds []*kind
	byName := map[string]*kind{}
	get := func(name string) *kind {
		if k := byName[name]; k != nil {
			return k
		}
		k := &kind{name: name, byKey: map[string]*key{}}
		byName[name] = k
		kinds = append(kinds, k)
		return k
	}
	var cur *kind
	var prose []string
	keyOf := func(k *kind, name string) *key {
		if x := k.byKey[name]; x != nil {
			return x
		}
		x := &key{name: name}
		k.byKey[name] = x
		k.keys = append(k.keys, x)
		return x
	}
	var doc *key       // the key an indented doc line describes
	var pending []*key // described keys not yet given to a kind
	adopt := func(k *kind) {
		for _, d := range pending {
			x := keyOf(k, d.name)
			if x.value == "" {
				x.value = d.value
			}
			if x.help == "" {
				x.help = d.help
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
				cur.named = true
			}
			if cur.help == "" && len(prose) > 0 {
				cur.help = strings.Join(prose, " ")
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
			doc = &key{name: m[1], value: unquote(m[2]), help: m[3]}
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
			doc.help = strings.TrimSpace(doc.help + sep + more)
			continue
		}
		if m := keyRE.FindStringSubmatch(line); m != nil && cur != nil {
			adopt(cur)
			x := keyOf(cur, m[2])
			v := unquote(m[3])
			if x.help == "" && len(prose) > 0 {
				x.help = strings.Join(prose, " ")
			}
			if x.value == "" && m[1] == "" {
				x.value = v
			} else if v != "" && v != x.value {
				seen := false
				for _, e := range x.examples {
					seen = seen || e == v
				}
				if !seen {
					x.examples = append(x.examples, v)
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
		if h, ok := kindHelp[k.name]; ok {
			k.help = h
		}
		if f, ok := freeKinds[k.name]; ok {
			k.free, k.keys = f, nil
		}
	}
	// a key with no value of its own: its first example is it
	for _, k := range kinds {
		for _, x := range k.keys {
			if x.value == "" && len(x.examples) > 0 {
				x.value, x.examples = x.examples[0], x.examples[1:]
			}
		}
	}
	return kinds
}

func header(k *kind) string {
	if k.named {
		return "[" + k.name + " NAME]"
	}
	return "[" + k.name + "]"
}

func anchor(k *kind) string {
	return strings.ToLower(strings.ReplaceAll(strings.Trim(header(k), "[]"), " ", "-"))
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.Join(strings.Fields(s), " ")
}

func code(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

func prose(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ")") && !strings.HasSuffix(s, ":") {
		s += "."
	}
	return s
}

func reference(kinds []*kind) string {
	var b strings.Builder
	b.WriteString(`<!-- Made by go run ./tools/confref from modemmenu.conf.sample: edit the sample, not this page. -->

# Configuration reference

Every section and key of modemmenu.conf, from the comments in
[modemmenu.conf.sample](https://github.com/D-Modem/menu/blob/main/modemmenu.conf.sample).
**Sample** is the value the sample sets (or shows, commented out);
[Configuration](configuration.md) explains the file and its sections in
full.

| section | what it is |
|---|---|
`)
	for _, k := range kinds {
		help := k.help
		if i := strings.Index(help, ". "); i >= 0 {
			help = help[:i+1]
		}
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s |\n", header(k), anchor(k), cell(prose(help)))
	}
	for _, k := range kinds {
		fmt.Fprintf(&b, "\n## `%s`\n\n", header(k))
		if k.help != "" {
			b.WriteString(prose(k.help) + "\n\n")
		}
		if k.named {
			b.WriteString("There may be any number of these, each with its own NAME.\n\n")
		}
		if k.free != "" {
			fmt.Fprintf(&b, "Each key is %s.\n", k.free)
		}
		if len(k.keys) == 0 {
			continue
		}
		b.WriteString("| key | sample | what it does |\n|---|---|---|\n")
		for _, x := range k.keys {
			help := prose(x.help)
			if help == "" {
				help = "See the section above."
			}
			if len(x.examples) > 0 {
				var ex []string
				for _, e := range x.examples {
					ex = append(ex, code(e))
				}
				help += " Also: " + strings.Join(ex, ", ") + "."
			}
			v := ""
			if x.value != "" {
				v = code(x.value)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", x.name, v, cell(help))
		}
	}
	b.WriteString("\n**See also:** [Configuration](configuration.md)\n")
	return b.String()
}

func main() {
	sample, err := os.ReadFile("modemmenu.conf.sample")
	if err != nil {
		log.Fatal(err)
	}
	out := "docs/reference.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, []byte(reference(parse(string(sample)))), 0o644); err != nil {
		log.Fatal(err)
	}
}
