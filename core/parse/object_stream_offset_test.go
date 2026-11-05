package parse

import (
	"os"
	"regexp"
	"testing"
)

const objStmFixture = "../../tests/resources/objstm_xrefstream.pdf"

// TestObjectStreamsResolveViaXrefOffset is the regression test for objects in
// compressed object streams being unreachable on real PDF 1.5+ files.
//
// GetObjectFromStream located the container by scanning for "N 0 obj" anchored
// at a start-of-line. Nothing in the spec requires an object header to begin a
// line, and on this fixture (a real FDA guidance PDF with cross-reference
// streams) it does not — so 31 of its 73 objects were unreachable, and every
// manipulate entry point, which builds its object map through this path, was
// unusable on the file.
//
// The xref already carried the container's byte offset; it was being thrown
// away. This test pins that every object the xref knows about is readable.
func TestObjectStreamsResolveViaXrefOffset(t *testing.T) {
	data, err := os.ReadFile(objStmFixture)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	pdf, err := OpenWithOptions(data, ParseOptions{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	objs := pdf.Objects()
	if len(objs) == 0 {
		t.Fatal("no objects parsed")
	}
	var inStream, failed int
	var firstErr error
	for _, n := range objs {
		if ref, ok := pdf.xref.Objects[n]; ok && ref.InStream {
			inStream++
		}
		if _, err := pdf.GetObjectContent(n); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if inStream == 0 {
		t.Fatal("fixture no longer exercises compressed object streams")
	}
	if failed > 0 {
		t.Errorf("%d of %d objects unreadable (%d are in object streams); first error: %v",
			failed, len(objs), inStream, firstErr)
	}
}

// The scan fallback must not depend on the header starting a line, and must
// not assume generation 0 — both were true of the original pattern.
func TestLocateObjectStreamFallback(t *testing.T) {
	// Header preceded by other content on the same line, and generation 1.
	raw := []byte(">>endobj 44 1 obj\n<</Type/ObjStm>>\n")
	off, err := locateObjectStream(raw, 44, 0)
	if err != nil {
		t.Fatalf("fallback did not find a non-line-anchored header: %v", err)
	}
	if !regexp.MustCompile(`^\s*44\s+1\s+obj`).Match(raw[off:]) {
		t.Errorf("offset %d does not point at the object header", off)
	}
}

// An xref offset that does not actually point at the object must be rejected
// rather than trusted — a stale offset would otherwise read a neighbouring
// object's bytes as if they were the container's.
func TestLocateObjectStreamRejectsWrongOffset(t *testing.T) {
	raw := []byte("%PDF-1.5\n7 0 obj\n<<>>\nendobj\n44 0 obj\n<</Type/ObjStm>>\n")
	good, err := locateObjectStream(raw, 44, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Offset 9 points at object 7, not 44; the locator must fall back and find
	// the real one instead of trusting it.
	got, err := locateObjectStream(raw, 44, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got != good {
		t.Errorf("a wrong offset was trusted: got %d, want the scanned %d", got, good)
	}
}

// The \b bound must still rule out the ambiguity the old ^ anchor existed for:
// looking for object 11 must not match the "11" inside "111 0 obj".
func TestLocateObjectStreamDoesNotMatchLongerObjectNumber(t *testing.T) {
	raw := []byte("%PDF-1.5\n111 0 obj\n<</Type/ObjStm>>\nendobj\n")
	if _, err := locateObjectStream(raw, 11, 0); err == nil {
		t.Error("object 11 was matched by the substring inside \"111 0 obj\"")
	}
}
