package parse

import (
	"bytes"
	"regexp"
	"strconv"
)

// streamKeywordPos returns the index of the "stream" keyword in an object
// body, or -1 for non-stream objects. The search starts after the object's
// dictionary: a naive bytes.Index would match "stream" inside dictionary
// names such as /Subtype /application#2Foctet-stream — or inside a string
// value like /F (data-stream.txt) — and misplace the stream data (or
// misclassify a plain dictionary as a stream object).
func streamKeywordPos(content []byte) int {
	from := 0
	if dictStart := bytes.Index(content, []byte("<<")); dictStart != -1 {
		depth := 0
		inString := false
		for i := dictStart; i < len(content); i++ {
			c := content[i]
			if inString {
				switch c {
				case '\\':
					i++ // skip escaped byte
				case ')':
					inString = false
				}
				continue
			}
			switch c {
			case '(':
				inString = true
			case '<':
				depth++
			case '>':
				depth--
				if depth == 0 {
					from = i + 1
					i = len(content)
				}
			}
		}
	}
	rel := bytes.Index(content[from:], []byte("stream"))
	if rel == -1 {
		return -1
	}
	return from + rel
}

// directStreamLength returns the stream length declared by a direct /Length
// entry in the stream dictionary, or 0 when /Length is absent or an indirect
// reference (/Length N G R). An indirect length cannot be resolved from the
// dictionary bytes alone, so callers fall back to a marker search.
func directStreamLength(dict []byte) int {
	m := lengthEntryPattern.FindSubmatch(dict)
	if m == nil || len(m[2]) > 0 {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

// lengthEntryPattern captures the /Length value (group 1) and, if the length
// is an indirect reference, the trailing "G R" (group 2) so callers can tell a
// direct integer from an object reference.
var lengthEntryPattern = regexp.MustCompile(`/Length\s+(\d+)(\s+\d+\s+R)?`)

// endstreamKeywordPos returns the index of the "endstream" keyword that closes
// the stream whose "stream" keyword sits at streamStart, or len(content) when
// no marker is found. It honours a direct /Length so the search skips past
// stream payload bytes that may themselves contain the literal "endstream";
// without a usable /Length it bounds the search to the start of the stream
// data, which still avoids spurious matches inside the dictionary.
func endstreamKeywordPos(content []byte, streamStart int) int {
	streamDataStart := streamStart + len("stream")
	if streamDataStart < len(content) && content[streamDataStart] == '\r' {
		streamDataStart++
	}
	if streamDataStart < len(content) && content[streamDataStart] == '\n' {
		streamDataStart++
	}

	searchFrom := streamDataStart
	if n := directStreamLength(content[:streamStart]); n > 0 && streamDataStart+n <= len(content) {
		searchFrom = streamDataStart + n
	}

	rel := bytes.Index(content[searchFrom:], []byte("endstream"))
	if rel == -1 {
		// A /Length larger than the real stream data (a common malformation,
		// e.g. one that counts a trailing EOL) pushes searchFrom past the real
		// "endstream". Fall back to a full scan from the stream data start so
		// the stream isn't silently dropped.
		if searchFrom > streamDataStart {
			if rel = bytes.Index(content[streamDataStart:], []byte("endstream")); rel != -1 {
				return streamDataStart + rel
			}
		}
		return len(content)
	}
	return searchFrom + rel
}
