// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// section is one [name] of the configuration, its keys in file order
type section struct {
	name string
	keys []string
	vals map[string]string
	line map[string]int
}

func (s *section) get(key, def string) string {
	if v, ok := s.vals[key]; ok {
		return v
	}
	return def
}

func (s *section) has(key string) bool {
	_, ok := s.vals[key]
	return ok
}

func (s *section) bool(key string, def bool) (bool, error) {
	v, ok := s.vals[key]
	if !ok {
		return def, nil
	}
	switch strings.ToLower(v) {
	case "yes", "true", "on", "1":
		return true, nil
	case "no", "false", "off", "0":
		return false, nil
	}
	return def, fmt.Errorf("[%s] %s: %q is not yes or no", s.name, key, v)
}

func (s *section) int(key string, def int) (int, error) {
	v, ok := s.vals[key]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("[%s] %s: %q is not a number", s.name, key, v)
	}
	return n, nil
}

// seconds: a plain number of seconds, or a Go duration (90s, 5m)
func (s *section) duration(key string, def time.Duration) (time.Duration, error) {
	v, ok := s.vals[key]
	if !ok {
		return def, nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def, fmt.Errorf("[%s] %s: %q is not a duration", s.name, key, v)
	}
	return d, nil
}

// ini is a dmodem.conf-style file: [sections] of key = value, ';' or '#'
// comments on lines of their own or after a value (following a space)
type ini struct {
	path     string
	sections []*section
	byName   map[string]*section
}

func (c *ini) section(name string) *section {
	if s, ok := c.byName[name]; ok {
		return s
	}
	return &section{name: name, vals: map[string]string{}, line: map[string]int{}}
}

func parseINI(path string) (*ini, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	c := &ini{path: path, byName: map[string]*section{}}
	var cur *section
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return nil, fmt.Errorf("%s:%d: no ] in %q", path, n, line)
			}
			name := strings.Join(strings.Fields(line[1:end]), " ")
			if s, ok := c.byName[name]; ok {
				cur = s
				continue
			}
			cur = &section{name: name, vals: map[string]string{}, line: map[string]int{}}
			c.sections = append(c.sections, cur)
			c.byName[name] = cur
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("%s:%d: no = in %q", path, n, line)
		}
		if cur == nil {
			return nil, fmt.Errorf("%s:%d: %q is before any [section]", path, n, line)
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(stripComment(line[eq+1:]))
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if _, ok := cur.vals[key]; !ok {
			cur.keys = append(cur.keys, key)
		}
		cur.vals[key] = val
		cur.line[key] = n
	}
	return c, sc.Err()
}

// stripComment: a value without the comment after it: from a ; or # that
// follows a space or tab, outside "quotes".  \; and \# are the characters
// themselves.
func stripComment(v string) string {
	var b strings.Builder
	quoted := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\' && i+1 < len(v) && (v[i+1] == ';' || v[i+1] == '#'):
			i++
			b.WriteByte(v[i])
			continue
		case c == '"':
			quoted = !quoted
		case (c == ';' || c == '#') && !quoted && (i == 0 || v[i-1] == ' ' || v[i-1] == '\t'):
			return b.String()
		}
		b.WriteByte(c)
	}
	return b.String()
}

// durations: a list of durations ("5m 1m", "300, 60"), the longest
// first, each once; "0" or "none" for none
func (s *section) durations(key, def string) ([]time.Duration, error) {
	v := strings.TrimSpace(s.get(key, def))
	if v == "" || v == "0" || strings.EqualFold(v, "none") {
		return nil, nil
	}
	var out []time.Duration
	for _, f := range fields(v) {
		one := &section{name: s.name, vals: map[string]string{key: f}}
		d, err := one.duration(key, 0)
		if err != nil {
			return nil, err
		}
		if d <= 0 {
			return nil, fmt.Errorf("[%s] %s: %q is not a time before the limit", s.name, key, f)
		}
		dup := false
		for _, o := range out {
			dup = dup || o == d
		}
		if !dup {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out, nil
}

// unescape turns \r \n \t \e \\ and \xHH of a configured string into
// their bytes
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'e':
			b.WriteByte(0x1b)
		case 'x':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 2
					continue
				}
			}
			b.WriteString("\\x")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func fields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
}
