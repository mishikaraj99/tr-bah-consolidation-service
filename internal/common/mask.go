package common

import "strings"

// MaskPhone hides all but the last four digits: "+919876543210" → "+91******3210".
func MaskPhone(p string) string {
	if p == "" {
		return ""
	}
	prefix := ""
	rest := p
	if strings.HasPrefix(rest, "+") {
		prefix = "+"
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "91") && len(rest) > 6 {
		prefix += "91"
		rest = rest[2:]
	}
	if len(rest) <= 4 {
		return prefix + strings.Repeat("*", len(rest))
	}
	return prefix + strings.Repeat("*", len(rest)-4) + rest[len(rest)-4:]
}
