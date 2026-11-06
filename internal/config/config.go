// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package config reads modemmenu.conf: [sections] of key = value, the
// values as strings, yes/no, numbers and durations, and the {name}
// templates of its strings.
package config

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Section is one [name] of the configuration, its keys in file order
type Section struct {
	Name string
	Keys []string
	Vals map[string]string
	Line map[string]int
}

// Get: key's value, or def when it is not set
func (s *Section) Get(key, def string) string {
	if v, ok := s.Vals[key]; ok {
		return v
	}
	return def
}

// Has: whether key is set
func (s *Section) Has(key string) bool {
	_, ok := s.Vals[key]
	return ok
}

// Bool: key as yes/no (true/false, on/off, 1/0), or def
func (s *Section) Bool(key string, def bool) (bool, error) {
	v, ok := s.Vals[key]
	if !ok {
		return def, nil
	}
	switch strings.ToLower(v) {
	case "yes", "true", "on", "1":
		return true, nil
	case "no", "false", "off", "0":
		return false, nil
	}
	return def, fmt.Errorf("[%s] %s: %q is not yes or no", s.Name, key, v)
}

// Int: key as a number, or def
func (s *Section) Int(key string, def int) (int, error) {
	v, ok := s.Vals[key]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("[%s] %s: %q is not a number", s.Name, key, v)
	}
	return n, nil
}

// seconds: a plain number of seconds, or a Go Duration (90s, 5m)
func (s *Section) Duration(key string, def time.Duration) (time.Duration, error) {
	v, ok := s.Vals[key]
	if !ok {
		return def, nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def, fmt.Errorf("[%s] %s: %q is not a duration", s.Name, key, v)
	}
	return d, nil
}

// File is a dmodem.conf-style file: [sections] of key = value, ';' or '#'
// comments on lines of their own or after a value (following a space)
type File struct {
	Path     string
	Sections []*Section
	ByName   map[string]*Section
}

// Section: the Section by name; an empty one if there is none
func (c *File) Section(name string) *Section {
	if s, ok := c.ByName[name]; ok {
		return s
	}
	return &Section{Name: name, Vals: map[string]string{}, Line: map[string]int{}}
}

// Parse: the configuration file's sections and keys (keys in lower
// case, inline comments and surrounding quotes taken off values)
func Parse(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	c := &File{Path: path, ByName: map[string]*Section{}}
	var cur *Section
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
			if s, ok := c.ByName[name]; ok {
				cur = s
				continue
			}
			cur = &Section{Name: name, Vals: map[string]string{}, Line: map[string]int{}}
			c.Sections = append(c.Sections, cur)
			c.ByName[name] = cur
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
		val := strings.TrimSpace(StripComment(line[eq+1:]))
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if _, ok := cur.Vals[key]; !ok {
			cur.Keys = append(cur.Keys, key)
		}
		cur.Vals[key] = val
		cur.Line[key] = n
	}
	return c, sc.Err()
}

// StripComment: a value without the comment after it: from a ; or # that
// follows a space or tab, outside "quotes".  \; and \# are the characters
// themselves.
func StripComment(v string) string {
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

// Durations: a list of Durations ("5m 1m", "300, 60"), the longest
// first, each once; "0" or "none" for none
func (s *Section) Durations(key, def string) ([]time.Duration, error) {
	v := strings.TrimSpace(s.Get(key, def))
	if v == "" || v == "0" || strings.EqualFold(v, "none") {
		return nil, nil
	}
	var out []time.Duration
	for _, f := range Fields(v) {
		one := &Section{Name: s.Name, Vals: map[string]string{key: f}}
		d, err := one.Duration(key, 0)
		if err != nil {
			return nil, err
		}
		if d <= 0 {
			return nil, fmt.Errorf("[%s] %s: %q is not a time before the limit", s.Name, key, f)
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

// Unescape turns \r \n \t \e \\ and \xHH of a configured string into
// their bytes
func Unescape(s string) string {
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

// Fields: a list split at commas and whitespace
func Fields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
}
