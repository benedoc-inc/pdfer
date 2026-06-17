package parse

import (
	"bytes"
	"regexp"
)

// crossRefContainerRE matches the /Type entry of an object stream
// (/Type/ObjStm) or a cross-reference stream (/Type/XRef). Spacing between the
// /Type key and its value is tolerated; the \b prevents matching longer names
// (e.g. /XRefStm is not /XRef). Compiled once at package scope.
var crossRefContainerRE = regexp.MustCompile(`/Type\s*/(ObjStm|XRef)\b`)

// IsCrossRefContainerObject reports whether an object body (as returned by
// GetObjectContent) is an object stream (/Type/ObjStm) or a cross-reference
// stream (/Type/XRef).
//
// This is hygiene, not a correctness fix. When a rebuild path flattens an
// object stream's members into standalone objects and the PDFWriter regenerates
// a fresh classical xref, the original container objects become dead weight —
// orphans nothing references. Re-emitting them just bloats the output (and can
// confuse lenient parsers that scan for /Type/ObjStm). Skipping them keeps
// rebuilt files clean. It does NOT fix the issue #26 round-trip data loss; that
// was a fixed-window truncation in the traditional-xref parser (see
// parseTraditionalxrefSection in incremental.go).
//
// The check is scoped to the dict portion (bytes before the first `stream`
// keyword) to avoid matching byte sequences inside binary stream data.
func IsCrossRefContainerObject(objBody []byte) bool {
	dict := objBody
	if idx := bytes.Index(objBody, []byte("stream")); idx >= 0 {
		dict = objBody[:idx]
	}
	return crossRefContainerRE.Match(dict)
}
