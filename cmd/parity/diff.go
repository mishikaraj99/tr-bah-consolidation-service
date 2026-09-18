package main

import (
	"fmt"
	"sort"
	"strings"
)

// Diff is one field-level difference between two responses.
type Diff struct {
	Path string
	A    any
	B    any
	Note string
}

func (d Diff) String() string {
	if d.Note != "" {
		return fmt.Sprintf("%s: %s (a=%v b=%v)", d.Path, d.Note, render(d.A), render(d.B))
	}
	return fmt.Sprintf("%s: a=%v b=%v", d.Path, render(d.A), render(d.B))
}

func render(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%#v", v)
}

// CompareJSON walks two decoded JSON values and reports every difference.
// Null and a missing key are DIFFERENT; array order matters. Paths in ignore are skipped.
func CompareJSON(a, b any, ignore map[string]bool) []Diff {
	var out []Diff
	compare("$", a, b, ignore, &out)
	return out
}

func ignored(path string, ignore map[string]bool) bool {
	if ignore[path] {
		return true
	}
	// also match the bare leaf name so callers can ignore "txnDate" anywhere
	if i := strings.LastIndexAny(path, ".["); i >= 0 {
		leaf := strings.TrimLeft(path[i:], ".[")
		leaf = strings.TrimRight(leaf, "]")
		if ignore[leaf] {
			return true
		}
	}
	return false
}

func compare(path string, a, b any, ignore map[string]bool, out *[]Diff) {
	if ignored(path, ignore) {
		return
	}
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, Diff{Path: path, A: a, B: b, Note: "type mismatch"})
			return
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			child := path + "." + k
			if ignored(child, ignore) {
				continue
			}
			aVal, aOK := av[k]
			bVal, bOK := bv[k]
			switch {
			case aOK && !bOK:
				*out = append(*out, Diff{Path: child, A: aVal, B: nil, Note: "missing in b"})
			case !aOK && bOK:
				*out = append(*out, Diff{Path: child, A: nil, B: bVal, Note: "missing in a"})
			default:
				compare(child, aVal, bVal, ignore, out)
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			*out = append(*out, Diff{Path: path, A: a, B: b, Note: "type mismatch"})
			return
		}
		if len(av) != len(bv) {
			*out = append(*out, Diff{Path: path, A: len(av), B: len(bv), Note: "length mismatch"})
		}
		n := len(av)
		if len(bv) < n {
			n = len(bv)
		}
		for i := 0; i < n; i++ {
			compare(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i], ignore, out)
		}
	default:
		if !equalScalar(a, b) {
			*out = append(*out, Diff{Path: path, A: a, B: b})
		}
	}
}

func equalScalar(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return af == bf
	}
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}
