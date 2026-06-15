package parse

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"
)

// TestStreamKeywordPos_SkipsDictNamesAndStrings verifies the keyword is located
// after the dictionary, so "stream" appearing inside a name or string value
// does not produce a spurious match.
func TestStreamKeywordPos_SkipsDictNamesAndStrings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		isPos   bool // true: keyword present (pos > -1); false: -1
	}{
		{
			name:    "name containing stream, real stream follows",
			content: "<</Subtype/application#2Foctet-stream/Length 5>>\nstream\nHELLO\nendstream",
			isPos:   true,
		},
		{
			name:    "string containing stream, no real stream",
			content: "<</Type/Filespec/F (data-stream.txt)>>",
			isPos:   false,
		},
		{
			name:    "name containing stream, no real stream",
			content: "<</Type/Foo/Mode/octet-stream>>",
			isPos:   false,
		},
		{
			name:    "plain stream",
			content: "<</Length 3>>\nstream\nabc\nendstream",
			isPos:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := streamKeywordPos([]byte(tt.content))
			if tt.isPos && pos == -1 {
				t.Fatalf("expected to find stream keyword, got -1")
			}
			if !tt.isPos && pos != -1 {
				t.Fatalf("expected no stream keyword, got pos %d (matched %q)", pos, tt.content[pos:])
			}
			if pos != -1 && !bytes.HasPrefix([]byte(tt.content)[pos:], []byte("stream")) {
				t.Fatalf("pos %d does not point at the stream keyword: %q", pos, tt.content[pos:])
			}
		})
	}
}

// TestDirectStreamLength distinguishes a direct integer /Length from an indirect
// reference, which cannot be resolved from the dictionary bytes alone.
func TestDirectStreamLength(t *testing.T) {
	tests := []struct {
		dict string
		want int
	}{
		{"<</Length 42>>", 42},
		{"<</Length   7/Filter/FlateDecode>>", 7},
		{"<</Length 9 0 R>>", 0}, // indirect — not resolvable here
		{"<</Filter/FlateDecode>>", 0},
	}
	for _, tt := range tests {
		if got := directStreamLength([]byte(tt.dict)); got != tt.want {
			t.Errorf("directStreamLength(%q) = %d, want %d", tt.dict, got, tt.want)
		}
	}
}

// TestEndstreamKeywordPos_SkipsPayloadMarker verifies that a /Length-bounded
// search skips an "endstream" literal embedded in the stream payload and lands
// on the marker that actually closes the stream.
func TestEndstreamKeywordPos_SkipsPayloadMarker(t *testing.T) {
	payload := "AAAendstreamBBB" // contains the literal marker
	content := []byte("<</Length 15>>\nstream\n" + payload + "\nendstream")

	streamStart := streamKeywordPos(content)
	if streamStart == -1 {
		t.Fatal("stream keyword not found")
	}
	pos := endstreamKeywordPos(content, streamStart)

	// The marker just before the embedded one would land mid-payload; the
	// /Length-bounded search must instead find the trailing marker.
	if got := content[pos:]; string(got) != "endstream" {
		t.Fatalf("endstreamKeywordPos landed on %q, want the trailing marker", got)
	}
	if firstMarker := bytes.Index(content, []byte("endstream")); pos == firstMarker {
		t.Fatalf("endstreamKeywordPos returned the embedded payload marker at %d", pos)
	}
}

// TestEndstreamKeywordPos_NoLengthFallback ensures that without a usable
// /Length the search still bounds to after the stream keyword and finds the
// first marker.
func TestEndstreamKeywordPos_NoLengthFallback(t *testing.T) {
	content := []byte("<</Filter/FlateDecode>>\nstream\nabcdef\nendstream")
	streamStart := streamKeywordPos(content)
	pos := endstreamKeywordPos(content, streamStart)
	if string(content[pos:]) != "endstream" {
		t.Fatalf("endstreamKeywordPos landed on %q", content[pos:])
	}
}

// TestParseXRefStreamFull_PayloadEndstreamMarker verifies the xref-stream
// parser does not truncate the compressed payload at an "endstream" literal
// inside it. The payload is stored (zlib NoCompression) so the literal bytes
// appear verbatim in the compressed stream; without the /Length-bounded search
// the stream is cut short and decompression fails.
func TestParseXRefStreamFull_PayloadEndstreamMarker(t *testing.T) {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.NoCompression)
	if _, err := zw.Write([]byte("endstream\x00")); err != nil { // contains the literal marker
		t.Fatalf("zlib write: %v", err)
	}
	zw.Close()
	comp := buf.Bytes()
	if !bytes.Contains(comp, []byte("endstream")) {
		t.Fatalf("test setup: compressed payload lacks the embedded marker")
	}

	var pdf []byte
	pdf = append(pdf, "%PDF-1.5\n"...)
	startXRef := len(pdf)
	pdf = append(pdf, fmt.Sprintf("1 0 obj\n<</Type/XRef/Size 2/W[1 2 2]/Length %d>>\nstream\n", len(comp))...)
	pdf = append(pdf, comp...)
	pdf = append(pdf, "\nendstream\nendobj\n"...)
	pdf = append(pdf, fmt.Sprintf("startxref\n%d\n%%%%EOF\n", startXRef)...)

	// err == nil proves the full payload was used: a truncated zlib stream would
	// fail to decompress.
	if _, err := ParseXRefStreamFull(pdf, int64(startXRef), false); err != nil {
		t.Fatalf("xref stream truncated at embedded endstream marker: %v", err)
	}
}

// TestParseRawObject_NonStreamDictWithStreamBytes verifies a plain dictionary
// whose values mention "stream" is not misclassified as a stream object and
// keeps its full dictionary.
func TestParseRawObject_NonStreamDictWithStreamBytes(t *testing.T) {
	body := []byte("<</Type/Filespec/F (data-stream.txt)/Desc (octet-stream blob)>>")
	pdf := buildSingleObjectPDF(5, body)
	offset := bytes.Index(pdf, []byte("5 0 obj"))

	obj, err := ParseRawObject(pdf, 5, 0, int64(offset))
	if err != nil {
		t.Fatalf("ParseRawObject: %v", err)
	}
	if obj.IsStream {
		t.Error("non-stream dictionary misclassified as stream")
	}
	if !bytes.Contains(obj.RawBytes, []byte("/Desc (octet-stream blob)")) {
		t.Errorf("dictionary truncated: %q", obj.RawBytes)
	}
}

// TestParseRawObject_StreamDictNameWithStreamBytes verifies a real stream object
// whose dictionary carries a name containing "stream" (the default attachment
// subtype) parses its dictionary and stream data correctly.
func TestParseRawObject_StreamDictNameWithStreamBytes(t *testing.T) {
	body := []byte("<</Type/EmbeddedFile/Subtype/application#2Foctet-stream/Length 5>>\nstream\nHELLO\nendstream")
	pdf := buildSingleObjectPDF(6, body)
	offset := bytes.Index(pdf, []byte("6 0 obj"))

	obj, err := ParseRawObject(pdf, 6, 0, int64(offset))
	if err != nil {
		t.Fatalf("ParseRawObject: %v", err)
	}
	if !obj.IsStream {
		t.Fatal("stream object not detected")
	}
	if string(obj.StreamRaw) != "HELLO" {
		t.Errorf("StreamRaw = %q, want HELLO", obj.StreamRaw)
	}
	if !bytes.Contains(obj.DictRaw, []byte("octet-stream")) || !bytes.Contains(obj.DictRaw, []byte("/Length 5")) {
		t.Errorf("dictionary not fully captured: %q", obj.DictRaw)
	}
}

// TestParseRawObject_PayloadEndstreamMarker verifies the byte-perfect path does
// not truncate StreamRaw at an "endstream" literal embedded in the payload.
func TestParseRawObject_PayloadEndstreamMarker(t *testing.T) {
	payload := "AAAendstreamBBB"
	body := []byte("<</Length 15>>\nstream\n" + payload + "\nendstream")
	pdf := buildSingleObjectPDF(8, body)
	offset := bytes.Index(pdf, []byte("8 0 obj"))

	obj, err := ParseRawObject(pdf, 8, 0, int64(offset))
	if err != nil {
		t.Fatalf("ParseRawObject: %v", err)
	}
	if string(obj.StreamRaw) != payload {
		t.Errorf("StreamRaw = %q, want %q (truncated at embedded marker)", obj.StreamRaw, payload)
	}
}

// TestGetObjectContent_PayloadEndstreamMarker verifies the non-encrypted path
// does not truncate a stream whose payload contains the literal bytes
// "endstream".
func TestGetObjectContent_PayloadEndstreamMarker(t *testing.T) {
	payload := "AAAendstreamBBB"
	body := []byte("<</Length 15>>\nstream\n" + payload + "\nendstream")
	pdf := buildSingleObjectPDF(7, body)

	got, err := GetObjectContent(pdf, 7, nil, false)
	if err != nil {
		t.Fatalf("GetObjectContent: %v", err)
	}
	if !bytes.Contains(got, []byte(payload)) {
		t.Errorf("stream truncated at embedded endstream marker; got %q", got)
	}
}
