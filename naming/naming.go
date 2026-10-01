// Package naming turns a name into what a directory and the tables of goddard
// can hold: lowercase, no accents, no punctuation, words joined by single
// dashes. It is the same rule for the projects and for the organizations,
// because both of them name a directory of the workspace.
package naming

import (
	"strings"
	"unicode"
)

// From is the slug of a name.
func From(name string) string {
	var builder strings.Builder
	dashed := false
	for _, r := range name {
		switch {
		case r == '\'' || r == '’':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dashed && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			dashed = false
			builder.WriteRune(plain(r))
		default:
			dashed = true
		}
	}
	return builder.String()
}

func plain(r rune) rune {
	r = unicode.ToLower(r)
	switch r {
	case 'á', 'à', 'ä', 'â', 'ã', 'å':
		return 'a'
	case 'é', 'è', 'ë', 'ê':
		return 'e'
	case 'í', 'ì', 'ï', 'î':
		return 'i'
	case 'ó', 'ò', 'ö', 'ô', 'õ':
		return 'o'
	case 'ú', 'ù', 'ü', 'û':
		return 'u'
	case 'ñ':
		return 'n'
	case 'ç':
		return 'c'
	}
	return r
}
