package xfa

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// structuralXFATypes are the XFA element types that form the subform "skeleton"
// a form packet mirrors. The form-state delta annotates these (and only these)
// with presence/access/etc.; everything else (ui/border/value/caption/exData/…)
// is chrome that the minimal delta omits — Acrobat re-derives it from the
// template.
var structuralXFATypes = map[string]bool{
	"subform": true, "subformSet": true, "exclGroup": true,
	"field": true, "draw": true, "area": true,
}

func isStructuralXFA(local string) bool { return structuralXFATypes[local] }

// NodeSeg is one segment of a NodePath: the identity of a structural node within
// its parent. Name is the node's XFA name. For an anonymous node Name is "" and
// Type carries the element type (e.g. "draw"), so the node stays addressable.
// Index is the 0-based position among siblings sharing this segment's key — the
// name for a named node, or "#"+Type for an anonymous one — under the same
// structural parent.
type NodeSeg struct {
	Name  string
	Type  string
	Index int
}

// key returns the canonical match key for a segment: "name[idx]" for a named
// node or "#type[idx]" for an anonymous one. It is the single internal encoding
// both the template walk and the override lookup go through, so there is no
// cross-party string convention left to drift. err is non-nil when the segment
// names neither a Name nor a Type.
func (s NodeSeg) key() (string, error) {
	switch {
	case s.Name != "":
		return s.Name + "[" + strconv.Itoa(s.Index) + "]", nil
	case s.Type != "":
		return "#" + s.Type + "[" + strconv.Itoa(s.Index) + "]", nil
	default:
		return "", fmt.Errorf("node segment has neither Name nor Type")
	}
}

// siblingKey is the key a segment is indexed under among its siblings: the bare
// name for a named node, or "#type" for an anonymous one. Two siblings share an
// index sequence iff they share this key — which is how the template walk and a
// re-parse of the emitted packet agree on a node's Index.
func (s NodeSeg) siblingKey() string {
	if s.Name != "" {
		return s.Name
	}
	return "#" + s.Type
}

// NodePath is the lossless structural identity of a form node: the ordered
// segments from the form root down to the node. Unlike a SOM dot-path it
// distinguishes repeating-group instances (via Index) and anonymous nodes (via
// Type), so two distinct nodes never collide on the same key. Construct one
// explicitly, or obtain one from ParseFormPacket; either way a caller never
// formats or parses a path string itself, which is what kept the prior
// slash-keyed API fragile across packages.
type NodePath []NodeSeg

// String renders the path in pdfer's canonical "/"-joined form
// ("Name[idx]" / "#Type[idx]" segments). It is a derived, display-and-match
// representation — callers build NodePaths structurally and never parse this
// back — so exposing it introduces no parsing convention. It is best-effort for
// a malformed segment (one with neither Name nor Type); BuildFormPacket rejects
// such segments up front via canonKey.
func (p NodePath) String() string {
	segs := make([]string, len(p))
	for i, s := range p {
		if k, err := s.key(); err == nil {
			segs[i] = k
		} else {
			segs[i] = "#" + s.Type // s.Name is "" here by definition of key's error
		}
	}
	return strings.Join(segs, "/")
}

// canonKey is the strict whole-path match key; it errors if the path is empty or
// any segment is malformed, so BuildFormPacket rejects an unusable override
// before emitting anything.
func (p NodePath) canonKey() (string, error) {
	if len(p) == 0 {
		return "", fmt.Errorf("empty node path")
	}
	segs := make([]string, len(p))
	for i, s := range p {
		k, err := s.key()
		if err != nil {
			return "", fmt.Errorf("segment %d: %w", i, err)
		}
		segs[i] = k
	}
	return strings.Join(segs, "/"), nil
}

// NodeOverride is the set of Form-DOM attribute deltas to stamp on one structural
// node, addressed by Path. Attrs holds true XML attributes — "presence"
// ("visible"|"hidden"|"invisible"), "access" ("open"|"readOnly"|"protected"),
// and any other attribute Acrobat honors directly on the node. Nested-element
// overrides (border/fill colour, caption, …) are not expressed here yet; when
// they are added it will be as a new field on this struct, leaving Attrs-only
// callers unchanged.
type NodeOverride struct {
	Path  NodePath
	Attrs map[string]string
}

// BuildOption configures BuildFormPacketWithOverrides. No options are defined yet;
// the type reserves an additive seam so a future build knob need not change the
// signature.
type BuildOption func(*buildOptions)

type buildOptions struct{}

// BuildFormPacketWithOverrides walks template and emits a minimal xfa-form/2.8
// form packet: the template's structural skeleton pruned to the nodes named in
// overrides (plus the ancestors needed to reach them), each annotated with its
// override's attributes. The root <form> checksum is left empty for
// SetXFAFormPacket to stamp. This reproduces the "presence-only skeleton" measured
// to restore a reveal byte-for-byte identically to Acrobat's own full delta,
// generalized to any attribute.
//
// Every override Path must resolve to exactly one template node; it errors
// otherwise, so a stale or mistyped path fails loudly rather than silently
// dropping a delta.
//
// This supersedes the presence-only BuildFormPacket: pass NodeOverrides carrying a
// "presence" attribute for the same effect, plus any other attribute Acrobat
// honors (access, locale, h, …).
func BuildFormPacketWithOverrides(template []byte, overrides []NodeOverride, opts ...BuildOption) ([]byte, error) {
	var cfg buildOptions
	for _, opt := range opts {
		opt(&cfg)
	}

	want, err := overrideIndex(overrides)
	if err != nil {
		return nil, err
	}
	return buildFormPacket(template, want)
}

// buildFormPacket is the shared core behind BuildFormPacketWithOverrides and the
// deprecated presence-only BuildFormPacket: it walks template, prunes to the nodes
// keyed in want (path string → attributes), and emits the packet. want is the same
// path-keyed match table both entry points produce, so neither has to reconstruct
// a NodePath from a string.
func buildFormPacket(template []byte, want map[string]map[string]string) ([]byte, error) {
	root := &genNode{typ: "#root"}
	stack := []*genNode{root}
	err := walkStructuralXFA(template, func(path NodePath, typ string, attrs map[string]string) {
		last := path[len(path)-1]
		n := &genNode{typ: typ, name: attrs["name"], path: path.String(), segKey: last.siblingKey(), idx: last.Index}
		parent := stack[len(stack)-1]
		parent.children = append(parent.children, n)
		stack = append(stack, n)
	}, func() {
		stack = stack[:len(stack)-1]
	})
	if err != nil {
		return nil, fmt.Errorf("walking template: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(`<form xmlns="http://www.xfa.org/schema/xfa-form/2.8/">`)
	matched := emitChildren(&buf, root.children, want)
	buf.WriteString(`</form>`)

	if matched != len(want) {
		return nil, fmt.Errorf("%d of %d override path(s) did not match any template node", len(want)-matched, len(want))
	}
	return buf.Bytes(), nil
}

// PresenceSet maps a structural node's path (in canonical "/"-joined
// "Name[idx]" / "#Type[idx]" form) to its restored presence
// ("visible"|"hidden"|"invisible").
//
// Deprecated: use NodeOverride, which addresses nodes with a typed NodePath
// instead of a hand-formatted string and carries any attribute, not just presence.
// Build a string key with NodePath.String() and read one back via ParseFormPacket.
type PresenceSet map[string]string

// BuildFormPacket emits a presence-only form packet from a path-keyed presence set.
//
// Deprecated: use BuildFormPacketWithOverrides, which takes typed NodePaths and any
// attribute. This shim simply wraps each presence value as a {"presence": v}
// override keyed by the same path string and is retained for source compatibility
// with v2.8.0.
func BuildFormPacket(template []byte, set PresenceSet) ([]byte, error) {
	want := make(map[string]map[string]string, len(set))
	for path, presence := range set {
		want[path] = map[string]string{"presence": presence}
	}
	return buildFormPacket(template, want)
}

// ParseFormPacketPresence extracts the presence set from a captured form packet:
// every structural node carrying a presence attribute, keyed by its path.
//
// Deprecated: use ParseFormPacket, which returns every attribute delta (not just
// presence) as typed NodeOverrides. This shim keeps only the presence attribute.
func ParseFormPacketPresence(formPacket []byte) (PresenceSet, error) {
	overrides, err := ParseFormPacket(formPacket)
	if err != nil {
		return nil, err
	}
	set := PresenceSet{}
	for _, o := range overrides {
		// Match the v2.8.0 surface: a node whose only presence delta is the empty
		// string contributes no entry, so the set's len/membership stay stable.
		if presence := o.Attrs["presence"]; presence != "" {
			set[o.Path.String()] = presence
		}
	}
	return set, nil
}

// overrideIndex keys overrides by their canonical path for O(1) lookup during the
// template walk, validating each: the path must be well-formed, carry at least
// one attribute, set no reserved or malformed attribute name, and not collide
// with another override.
func overrideIndex(overrides []NodeOverride) (map[string]map[string]string, error) {
	want := make(map[string]map[string]string, len(overrides))
	for i, o := range overrides {
		key, err := o.Path.canonKey()
		if err != nil {
			return nil, fmt.Errorf("override %d: %w", i, err)
		}
		if len(o.Attrs) == 0 {
			return nil, fmt.Errorf("override %d (%s): no attributes to set", i, key)
		}
		for name := range o.Attrs {
			if err := validateAttrName(name); err != nil {
				return nil, fmt.Errorf("override %d (%s): %w", i, key, err)
			}
		}
		if _, dup := want[key]; dup {
			return nil, fmt.Errorf("override %d: duplicate path %s", i, key)
		}
		want[key] = o.Attrs
	}
	return want, nil
}

// validateAttrName rejects attribute names that would produce a malformed packet:
// the empty name, "name" (a node's identity is emitted from its Path, not Attrs),
// and any name with characters not allowed in an XML attribute name. The character
// set is deliberately conservative — XFA attribute names are simple identifiers — so
// callers cannot smuggle structure-breaking text into a name. An interior ':' is
// allowed so a namespace-prefixed attribute (e.g. xml:lang, as surfaced by
// ParseFormPacket) round-trips back through the builder.
func validateAttrName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("empty attribute name")
	case name == "name":
		return fmt.Errorf("attribute %q is reserved (node identity comes from Path)", name)
	}
	for i, r := range name {
		ok := r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && (r == '-' || r == '.' || r == ':' || (r >= '0' && r <= '9')))
		if !ok {
			return fmt.Errorf("attribute name %q has an invalid character %q", name, r)
		}
	}
	return nil
}

// genNode is a structural template node retained for emitting the pruned packet.
// segKey/idx are the node's sibling key and 0-based index under its parent, used
// to keep positional alignment when pruning (see emitChildren).
type genNode struct {
	typ, name, path, segKey string
	idx                     int
	children                []*genNode
}

// subtreeOverrides counts the overrides that fall on n or any of its descendants.
// A subtree with zero is a candidate for pruning.
func subtreeOverrides(n *genNode, want map[string]map[string]string) int {
	count := 0
	if _, ok := want[n.path]; ok {
		count++
	}
	for _, c := range n.children {
		count += subtreeOverrides(c, want)
	}
	return count
}

// emitChildren writes the children that must appear in the packet and returns the
// number of overrides emitted among them. A child is written when its subtree
// carries an override, OR when it shares a sibling key with a later kept child
// and a lower index — an empty placeholder that preserves positional indexing so
// a re-parse reads anonymous and repeating-instance nodes at their true Index.
// (Named-unique nodes have one sibling per key, so they never trigger a
// placeholder and the minimal output is unchanged for them.)
func emitChildren(buf *bytes.Buffer, children []*genNode, want map[string]map[string]string) int {
	// highestKept[key] is the largest index among children of that sibling key
	// whose subtree carries an override; -1 (absent) means none are kept.
	highestKept := map[string]int{}
	kept := make([]bool, len(children))
	for i, c := range children {
		if subtreeOverrides(c, want) > 0 {
			kept[i] = true
			if hi, ok := highestKept[c.segKey]; !ok || c.idx > hi {
				highestKept[c.segKey] = c.idx
			}
		}
	}

	total := 0
	for i, c := range children {
		hi, hasKept := highestKept[c.segKey]
		if kept[i] || (hasKept && c.idx < hi) {
			total += renderNode(buf, c, want)
		}
	}
	return total
}

// renderNode unconditionally writes n (open tag, name, override attributes) and
// its emitted children, returning the number of overrides written in its subtree.
// A node with no override and no kept descendants renders as an empty placeholder.
func renderNode(buf *bytes.Buffer, n *genNode, want map[string]map[string]string) int {
	attrs, has := want[n.path]

	var inner bytes.Buffer
	count := emitChildren(&inner, n.children, want)

	buf.WriteByte('<')
	buf.WriteString(n.typ)
	if n.name != "" {
		buf.WriteString(` name="`)
		// Standard XML attribute escaping is sufficient here: the emitted packet
		// only needs to be valid XML that Acrobat re-parses (it does not feed the
		// form checksum, which is computed over template+datasets). EscapeText to a
		// bytes.Buffer never errors.
		_ = xml.EscapeText(buf, []byte(n.name))
		buf.WriteByte('"')
	}
	if has {
		writeSortedAttrs(buf, attrs)
		count++
	}
	buf.WriteByte('>')
	buf.Write(inner.Bytes())
	buf.WriteString("</")
	buf.WriteString(n.typ)
	buf.WriteByte('>')

	return count
}

// writeSortedAttrs writes attrs to buf in sorted name order so the emitted packet
// is deterministic regardless of the override map's iteration order.
func writeSortedAttrs(buf *bytes.Buffer, attrs map[string]string) {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		buf.WriteByte(' ')
		buf.WriteString(name)
		buf.WriteString(`="`)
		_ = xml.EscapeText(buf, []byte(attrs[name]))
		buf.WriteByte('"')
	}
}

// ParseFormPacket reads a captured form packet back into the overrides it
// encodes: every structural node carrying at least one attribute other than its
// name, keyed by its NodePath. It is the inverse of BuildFormPacket — building
// from ParseFormPacket(p)'s output reproduces p's attribute deltas — and is
// useful for inspecting or diffing an Acrobat-captured packet. Node values
// (nested <value>/<exData> subtrees) are not attributes and so are not returned.
func ParseFormPacket(formPacket []byte) ([]NodeOverride, error) {
	var out []NodeOverride
	err := walkStructuralXFA(formPacket, func(path NodePath, typ string, attrs map[string]string) {
		over := make(map[string]string, len(attrs))
		for name, val := range attrs {
			if name == "name" {
				continue
			}
			over[name] = val
		}
		if len(over) == 0 {
			return
		}
		out = append(out, NodeOverride{Path: path, Attrs: over})
	}, func() {})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// qualifiedAttrName reconstructs an attribute's name as it appeared in the
// source. walkStructuralXFA decodes with RawToken (namespaces unresolved), so a
// prefixed attribute like xml:lang arrives as {Space:"xml", Local:"lang"}; keying
// by Local alone would drop the prefix and collide xml:lang with a bare lang
// (last-write-wins). Returning "prefix:local" keeps the name lossless, so
// ParseFormPacket round-trips through BuildFormPacketWithOverrides.
func qualifiedAttrName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// walkStructuralXFA streams an XFA packet (template or form), invoking onEnter on
// every structural element with its computed NodePath (a fresh slice the callback
// may retain), element type, and all of its XML attributes, and onExit when it
// closes. Non-structural elements are skipped but their nesting is tracked so
// structural depth — and therefore sibling indexing — stays correct.
func walkStructuralXFA(data []byte, onEnter func(path NodePath, typ string, attrs map[string]string), onExit func()) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	// seenStack[i] counts same-key children at structural depth i so siblings get
	// stable indices; segs is the current structural path.
	seenStack := []map[string]int{{}}
	var segs NodePath

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			if !isStructuralXFA(local) {
				continue
			}
			attrs := make(map[string]string, len(t.Attr))
			for _, a := range t.Attr {
				attrs[qualifiedAttrName(a.Name)] = a.Value
			}
			name := attrs["name"]
			key := name
			if key == "" {
				key = "#" + local
			}
			seen := seenStack[len(seenStack)-1]
			idx := seen[key]
			seen[key]++
			segs = append(segs, NodeSeg{Name: name, Type: local, Index: idx})
			onEnter(append(NodePath(nil), segs...), local, attrs)
			seenStack = append(seenStack, map[string]int{})
		case xml.EndElement:
			if !isStructuralXFA(t.Name.Local) {
				continue
			}
			seenStack = seenStack[:len(seenStack)-1]
			segs = segs[:len(segs)-1]
			onExit()
		}
	}
}
