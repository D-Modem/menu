// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import "strings"

// Expand replaces {name} by its value, quote() of it (nil: as is);
// unknown names stay as they are
func Expand(t string, v map[string]string, quote func(string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(t, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(t[i:], '}')
		if j < 0 {
			break
		}
		name := t[i+1 : i+j]
		val, ok := v[strings.ToLower(name)]
		b.WriteString(t[:i])
		switch {
		case !ok:
			b.WriteString(t[i : i+j+1])
		case quote != nil:
			b.WriteString(quote(val))
		default:
			b.WriteString(val)
		}
		t = t[i+j+1:]
	}
	b.WriteString(t)
	return b.String()
}
