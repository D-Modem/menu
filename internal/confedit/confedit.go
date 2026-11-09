// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package confedit changes modemmenu.conf as a sysop would in an editor:
// a key's value on its own line (its comment after it kept), a new key
// after the last of its section, a new section at the end.  The comments,
// the blank lines and the order of everything else stay as they are.
package confedit

import (
	"strings"

	"github.com/D-Modem/menu/internal/config"
)

// Doc is a configuration file's lines.
type Doc struct {
	lines []string
}

// Parse is a Doc of a file's text.
func Parse(text string) *Doc {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	d := &Doc{}
	if text != "" {
		d.lines = strings.Split(text, "\n")
	}
	return d
}

// String is the file's text.
func (d *Doc) String() string {
	if len(d.lines) == 0 {
		return ""
	}
	return strings.Join(d.lines, "\n") + "\n"
}

// header is the section a line opens ("" and false: none).
func header(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || t[0] != '[' {
		return "", false
	}
	end := strings.IndexByte(t, ']')
	if end < 0 {
		return "", false
	}
	return strings.Join(strings.Fields(t[1:end]), " "), true
}

// keyLine is a line's key and its raw value (after =), ok false for a
// comment, a blank line or a header.
func keyLine(line string) (key, raw string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == ';' || t[0] == '#' || t[0] == '[' {
		return "", "", false
	}
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:eq])), line[eq+1:], true
}

// value is a raw value as the configuration reads it.
func value(raw string) string {
	v := strings.TrimSpace(config.StripComment(raw))
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}

// span is where a section is: its header's line, and the line after its
// last key (or after the header).
type span struct{ header, lastKey int }

// spans are each [name] of the file, in order (a name may come more than
// once: the configuration takes them as one).
func (d *Doc) spans(name string) []span {
	var out []span
	cur := ""
	in := false
	for i, l := range d.lines {
		if h, ok := header(l); ok {
			cur, in = h, h == name
			if in {
				out = append(out, span{i, i + 1})
			}
			continue
		}
		if _, _, ok := keyLine(l); ok && in && cur == name {
			out[len(out)-1].lastKey = i + 1
		}
	}
	return out
}

// Sections are the sections' names, in the order they first come.
func (d *Doc) Sections() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range d.lines {
		if h, ok := header(l); ok && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// Has is whether the file has [name].
func (d *Doc) Has(name string) bool { return len(d.spans(name)) > 0 }

// find is the line of a section's key (its last, which is the one that
// counts), -1 none.
func (d *Doc) find(section, key string) int {
	at := -1
	for _, sp := range d.spans(section) {
		for i := sp.header + 1; i < len(d.lines); i++ {
			if _, ok := header(d.lines[i]); ok {
				break
			}
			if k, _, ok := keyLine(d.lines[i]); ok && k == key {
				at = i
			}
		}
	}
	return at
}

// Keys are a section's keys, in file order, each once.
func (d *Doc) Keys(section string) []string {
	var out []string
	seen := map[string]bool{}
	for _, sp := range d.spans(section) {
		for i := sp.header + 1; i < len(d.lines); i++ {
			if _, ok := header(d.lines[i]); ok {
				break
			}
			if k, _, ok := keyLine(d.lines[i]); ok && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// Get is a key's value, and whether it is set.
func (d *Doc) Get(section, key string) (string, bool) {
	i := d.find(section, key)
	if i < 0 {
		return "", false
	}
	_, raw, _ := keyLine(d.lines[i])
	return value(raw), true
}

// Quote is v as it is written after "key = ": in quotes when it would
// not read back the same without.
func Quote(v string) string {
	if v == "" {
		return `""`
	}
	if strings.TrimSpace(v) != v || value(v) != v {
		return `"` + v + `"`
	}
	return v
}

// commentAt is where a raw value's comment begins (len(raw): none).
func commentAt(raw string) int {
	quoted := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '\\' && i+1 < len(raw) && (raw[i+1] == ';' || raw[i+1] == '#'):
			i++
		case c == '"':
			quoted = !quoted
		case (c == ';' || c == '#') && !quoted && (i == 0 || raw[i-1] == ' ' || raw[i-1] == '\t'):
			// the spaces before the comment go with it
			j := i
			for j > 0 && (raw[j-1] == ' ' || raw[j-1] == '\t') {
				j--
			}
			return j
		}
	}
	return len(raw)
}

// Set gives a key a value: on its line if it has one (its comment kept,
// and its quotes if it had them), else on a new line after the section's
// last key (the section added at the end if there is none).
func (d *Doc) Set(section, key, v string) {
	q := Quote(v)
	if i := d.find(section, key); i >= 0 {
		l := d.lines[i]
		eq := strings.IndexByte(l, '=')
		raw := l[eq+1:]
		c := commentAt(raw)
		old := strings.TrimSpace(raw[:c])
		if strings.HasPrefix(old, `"`) && !strings.HasPrefix(q, `"`) {
			q = `"` + v + `"`
		}
		lead := " "
		if len(raw) > 0 && raw[0] != ' ' && raw[0] != '\t' {
			lead = ""
		}
		d.lines[i] = l[:eq+1] + lead + q + raw[c:]
		return
	}
	sps := d.spans(section)
	if len(sps) == 0 {
		d.AddSection(section)
		sps = d.spans(section)
	}
	at := sps[len(sps)-1].lastKey
	d.insert(at, key+" = "+q)
}

// Unset takes a key out (every line of it): its default again.
func (d *Doc) Unset(section, key string) {
	for {
		i := d.find(section, key)
		if i < 0 {
			return
		}
		d.lines = append(d.lines[:i], d.lines[i+1:]...)
	}
}

// AddSection puts [name] at the end, after a blank line.
func (d *Doc) AddSection(name string) {
	if d.Has(name) {
		return
	}
	if n := len(d.lines); n > 0 && strings.TrimSpace(d.lines[n-1]) != "" {
		d.lines = append(d.lines, "")
	}
	d.lines = append(d.lines, "["+name+"]")
}

// RemoveSection takes out [name] and its keys, and the comments among
// them; the comment just before the next section stays with that one.
func (d *Doc) RemoveSection(name string) {
	for {
		sps := d.spans(name)
		if len(sps) == 0 {
			return
		}
		sp := sps[len(sps)-1]
		end := len(d.lines)
		for i := sp.header + 1; i < len(d.lines); i++ {
			if _, ok := header(d.lines[i]); ok {
				end = i
				break
			}
		}
		// the next section's own comment and the blank line before it
		if end < len(d.lines) {
			for end > sp.header+1 && isComment(d.lines[end-1]) {
				end--
			}
		}
		for end > sp.header+1 && strings.TrimSpace(d.lines[end-1]) == "" {
			end--
		}
		start := sp.header
		// its own comment, just above it
		for start > 0 && isComment(d.lines[start-1]) {
			start--
		}
		if start > 0 && strings.TrimSpace(d.lines[start-1]) == "" && (end >= len(d.lines) || strings.TrimSpace(d.lines[end]) == "") {
			start--
		}
		d.lines = append(d.lines[:start], d.lines[end:]...)
	}
}

// RenameSection gives [old] (each of its headers) a new name.
func (d *Doc) RenameSection(old, name string) {
	for _, sp := range d.spans(old) {
		d.lines[sp.header] = "[" + name + "]"
	}
}

func isComment(l string) bool {
	t := strings.TrimSpace(l)
	return t != "" && (t[0] == ';' || t[0] == '#')
}

func (d *Doc) insert(at int, line string) {
	d.lines = append(d.lines, "")
	copy(d.lines[at+1:], d.lines[at:])
	d.lines[at] = line
}
