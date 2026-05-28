package xfa

import (
	"fmt"
	"strings"
)

// somSegment returns the SOM-formatted name segment for child under parent.
// When more than one of parent's children shares child.Name, the segment is
// suffixed with "[i]" where i is child's 0-based document order among those
// same-named siblings — required by the XFA spec to disambiguate references.
// When child.Name is unique among parent's children, the bare name is returned.
//
// Anonymous children (Name == "") are excluded from the count: they are not
// SOM-addressable and would otherwise perturb sibling indexing.
//
// parent may be nil for top-level nodes; in that case the bare name is returned.
func somSegment(parent, child *xfaNode) string {
	if child == nil || child.Name == "" {
		return ""
	}
	if parent == nil {
		return child.Name
	}
	index := -1
	total := 0
	for _, sib := range parent.Children {
		if sib == nil || sib.Name != child.Name {
			continue
		}
		if sib == child {
			index = total
		}
		total++
	}
	if total <= 1 {
		return child.Name
	}
	if index < 0 {
		// child wasn't found in parent.Children — fall back to bare name
		// rather than emitting a misleading index.
		return child.Name
	}
	return fmt.Sprintf("%s[%d]", child.Name, index)
}

// somSegmentFromSiblingNames returns the SOM-formatted segment for the i-th
// entry in a slice of sibling names. Used for collections (e.g. flattened
// exclGroup option fields) where the sibling xfaNodes are not retained and
// only their names survive as a parallel slice. Empty names yield "".
func somSegmentFromSiblingNames(siblings []string, i int) string {
	if i < 0 || i >= len(siblings) {
		return ""
	}
	name := siblings[i]
	if name == "" {
		return ""
	}
	index := 0
	total := 0
	for j, n := range siblings {
		if n != name {
			continue
		}
		if j == i {
			index = total
		}
		total++
	}
	if total <= 1 {
		return name
	}
	return fmt.Sprintf("%s[%d]", name, index)
}

// somJoin assembles a SOM path from already-formatted segments, dropping any
// empty segments so callers don't have to pre-filter. This is the only place
// SOM segments get glued into a path string.
func somJoin(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ".")
}

// somAppend returns a new path slice with seg appended. seg is skipped when
// empty so anonymous nodes don't introduce empty path segments. The input
// slice is not mutated.
func somAppend(path []string, seg string) []string {
	if seg == "" {
		return path
	}
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}
