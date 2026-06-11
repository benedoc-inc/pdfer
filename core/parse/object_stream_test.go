package parse

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"
)

// TestGetObjectFromStream_HeaderBoundary verifies the object-stream header
// search: the stream object number must not match inside a longer number
// ("11 0 obj" inside "111 0 obj"), and a header preceded by a bare CR — a
// legal PDF EOL that a (?m)^ line anchor does not match after — must still be
// found.
func TestGetObjectFromStream_HeaderBoundary(t *testing.T) {
	// ObjStm payload: header "12 0 " (object 12 at offset 0), /First 5.
	payload := []byte("12 0 <</Foo/Bar>>")
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	zw.Write(payload)
	zw.Close()

	var pdf []byte
	pdf = append(pdf, "%PDF-1.5\n"...)
	// Decoy: a longer object number containing "11 0 obj" as a substring,
	// placed before the real object stream.
	pdf = append(pdf, "111 0 obj\n<</Type/Decoy>>\nendobj"...)
	// Real object stream, preceded by a bare CR instead of LF.
	pdf = append(pdf, '\r')
	pdf = append(pdf, fmt.Sprintf("11 0 obj\n<</Type/ObjStm/N 1/First 5/Length %d/Filter/FlateDecode>>\nstream\n", compressed.Len())...)
	pdf = append(pdf, compressed.Bytes()...)
	pdf = append(pdf, "\nendstream\nendobj\n%%EOF\n"...)

	got, err := GetObjectFromStream(pdf, 12, 11, 0, nil, false)
	if err != nil {
		t.Fatalf("GetObjectFromStream: %v", err)
	}
	if !bytes.Contains(got, []byte("/Foo/Bar")) {
		t.Errorf("object 12 content = %q, want it to contain %q", got, "/Foo/Bar")
	}
}
