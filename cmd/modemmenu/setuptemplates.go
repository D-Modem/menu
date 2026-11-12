// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"regexp"
	"strings"

	"github.com/D-Modem/menu/internal/confdoc"
)

// itemTemplate is a menu item of the sample's (set or commented out), to
// start a new [item] from: "Add a menu item" offers each.
type itemTemplate struct {
	name  string
	about string      // the sample's comment before it, or its label
	keys  [][2]string // its keys, in order
}

var (
	tplHeaderRE = regexp.MustCompile(`^;?\[item (\S+)\]$`)
	tplKeyRE    = regexp.MustCompile(`^;?([a-z][a-z0-9_]*) = (.*)$`)
)

// itemTemplates: the sample's items, each once (by its label)
func itemTemplates(sample string) []itemTemplate {
	var out []itemTemplate
	var prose []string
	var cur *itemTemplate
	alt := false
	seen := map[string]bool{}
	flush := func() {
		if cur != nil && len(cur.keys) > 0 {
			label := cur.get("label")
			if !seen[label] {
				seen[label] = true
				if cur.about == "" {
					cur.about = label
				}
				out = append(out, *cur)
			}
		}
		cur = nil
	}
	for _, raw := range strings.Split(sample, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" {
			flush()
			prose = nil
			continue
		}
		if m := tplHeaderRE.FindStringSubmatch(line); m != nil {
			flush()
			cur, alt = &itemTemplate{name: m[1]}, false
			if len(prose) > 0 {
				cur.about = capFirst(strings.Join(prose, " "))
			}
			prose = nil
			continue
		}
		if strings.HasPrefix(line, "[") {
			flush()
			prose = nil
			continue
		}
		if cur != nil {
			if m := tplKeyRE.FindStringSubmatch(line); m != nil {
				// after a comment in an item, the commented keys are its
				// alternatives ("; or a BBS that ..."), not its own
				if !(alt && strings.HasPrefix(line, ";")) {
					cur.keys = append(cur.keys, [2]string{m[1], confdoc.Unquote(m[2])})
				}
				continue
			}
			alt = true
			continue
		}
		if strings.HasPrefix(line, ";") {
			prose = append(prose, strings.TrimSpace(strings.TrimLeft(line, ";")))
		}
	}
	flush()
	return out
}

func (t *itemTemplate) get(key string) string {
	for _, kv := range t.keys {
		if kv[0] == key {
			return kv[1]
		}
	}
	return ""
}
