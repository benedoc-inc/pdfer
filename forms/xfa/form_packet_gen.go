package xfa

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// structuralXFATypes are the XFA element types that form the subform "skeleton"
// a form packet mirrors. The form-state delta annotates these (and only these)
// with presence; everything else (ui/border/value/caption/exData/…) is chrome
// that the minimal delta omits — Acrobat re-derives it from the template.
var structuralXFATypes = map[string]bool{
	"subform": true, "subformSet": true, "exclGroup": true,
	"field": true, "draw": true, "area": true,
}

func isStructuralXFA(local string) bool { return structuralXFATypes[local] }

// PresenceSet maps a structural node's path to its restored presence
// ("visible"|"hidden"|"invisible"). The path is "/"-joined segments from the
// form root; each segment is name[idx] (or #type[idx] when the node is unnamed),
// where idx is the 0-based position among same-key siblings under the same
// structural parent. walkStructuralXFA produces these keys identically for the
// template and for a captured form packet, so a set parsed from one aligns with
// a walk of the other.
type PresenceSet map[string]string

// walkStructuralXFA streams an XFA packet (template or form), invoking onEnter on
// every structural element with its computed path, type, name, and authored
// presence (""=unset), and onExit when it closes. Non-structural elements are
// skipped but their nesting is tracked so structural depth stays correct.
func walkStructuralXFA(data []byte, onEnter func(path, typ, name, presence string), onExit func()) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	// seenStack[i] counts same-key children at structural depth i so siblings get
	// stable indices; segs is the current structural path.
	seenStack := []map[string]int{{}}
	var segs []string

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
			var name, presence string
			for _, a := range t.Attr {
				switch a.Name.Local {
				case "name":
					name = a.Value
				case "presence":
					presence = a.Value
				}
			}
			key := name
			if key == "" {
				key = "#" + local
			}
			seen := seenStack[len(seenStack)-1]
			idx := seen[key]
			seen[key]++
			segs = append(segs, key+"["+strconv.Itoa(idx)+"]")
			onEnter(strings.Join(segs, "/"), local, name, presence)
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

// ParseFormPacketPresence extracts the presence set from a captured form packet:
// every structural node carrying a presence attribute, keyed by its path.
func ParseFormPacketPresence(formPacket []byte) (PresenceSet, error) {
	set := PresenceSet{}
	err := walkStructuralXFA(formPacket, func(path, typ, name, presence string) {
		if presence != "" {
			set[path] = presence
		}
	}, func() {})
	if err != nil {
		return nil, err
	}
	return set, nil
}

// genNode is a structural template node retained for emitting the pruned packet.
type genNode struct {
	typ, name, path string
	children        []*genNode
}

// BuildFormPacket walks the template and emits a minimal xfa-form/2.8 form packet:
// the template's structural tree pruned to nodes named in set (plus the ancestors
// needed to reach them), each emitted as <type name=.. [presence=..]>. The
// checksum attribute is left empty; SetXFAFormPacket stamps it. This reproduces
// the "presence-only skeleton" that was measured to restore a reveal byte-for-byte
// identically to Acrobat's own full delta.
func BuildFormPacket(template []byte, set PresenceSet) ([]byte, error) {
	root := &genNode{typ: "#root"}
	stack := []*genNode{root}

	err := walkStructuralXFA(template, func(path, typ, name, presence string) {
		n := &genNode{typ: typ, name: name, path: path}
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
	emitted := 0
	for _, top := range root.children {
		emitted += emitGenNode(&buf, top, set)
	}
	buf.WriteString(`</form>`)

	if emitted != len(set) {
		return nil, fmt.Errorf("presence set has %d entries but %d matched template paths", len(set), emitted)
	}
	return buf.Bytes(), nil
}

// emitGenNode writes n and its kept descendants, returning the number of presence
// entries emitted in this subtree. A node is kept when it carries a presence or
// has a kept descendant; subtrees with neither are pruned entirely.
func emitGenNode(buf *bytes.Buffer, n *genNode, set PresenceSet) int {
	presence, has := set[n.path]

	var inner bytes.Buffer
	childCount := 0
	for _, c := range n.children {
		childCount += emitGenNode(&inner, c, set)
	}

	if !has && childCount == 0 {
		return 0 // prune: nothing restored in this subtree
	}

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
		buf.WriteString(` presence="`)
		_ = xml.EscapeText(buf, []byte(presence))
		buf.WriteByte('"')
	}
	buf.WriteByte('>')
	buf.Write(inner.Bytes())
	buf.WriteString("</")
	buf.WriteString(n.typ)
	buf.WriteByte('>')

	if has {
		return childCount + 1
	}
	return childCount
}
