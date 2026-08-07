package incremental

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// buildPDF builds a minimal one-page PDF, with either a classical xref table
// or a cross-reference stream depending on useXRefStream.
func buildPDF(t *testing.T, useXRefStream bool) []byte {
	t.Helper()
	w := write.NewPDFWriter()
	w.UseXRefStream(useXRefStream)

	catalogNum := w.AddObject([]byte("<</Type/Catalog/Pages 2 0 R>>"))
	w.SetRoot(catalogNum)
	pagesNum := w.AddObject([]byte("<</Type/Pages/Count 0/Kids[]>>"))
	w.SetObject(catalogNum, []byte(fmt.Sprintf("<</Type/Catalog/Pages %d 0 R>>", pagesNum)))

	var buf bytes.Buffer
	if err := w.Write(&buf); err != nil {
		t.Fatalf("write source PDF: %v", err)
	}
	return buf.Bytes()
}

// appendStream runs one incremental update against original, appending a new
// stream object, and returns the result plus the appended object's number.
func appendStream(t *testing.T, original []byte) ([]byte, int) {
	t.Helper()
	pdf, err := parse.Open(original)
	if err != nil {
		t.Fatalf("parse source PDF: %v", err)
	}
	upd, err := New(original, pdf.Trailer(), pdf.Encryption())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	num := upd.Reserve()
	if err := upd.AddStream(num, 0, []byte("<<>>"), []byte("hello world")); err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	out, err := upd.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return out, num
}

// TestXRefStreamSource_EmitsXRefStream verifies that an incremental update
// against a PDF whose active xref is a cross-reference stream appends a
// cross-reference stream, not a classical table (ISO 32000-1 §7.5.8.4), and
// that the result still parses with the appended object reachable.
func TestXRefStreamSource_EmitsXRefStream(t *testing.T) {
	original := buildPDF(t, true)
	out, num := appendStream(t, original)

	if !bytes.HasPrefix(out, original) {
		t.Error("incremental update must preserve the original bytes as a prefix")
	}
	appended := out[len(original):]
	if bytes.Contains(appended, []byte("trailer")) {
		t.Error("appended revision must not contain a classical trailer")
	}
	if !bytes.Contains(appended, []byte("/Type/XRef")) {
		t.Error("appended revision must contain a cross-reference stream")
	}
	if !bytes.Contains(appended, []byte("/Prev")) {
		t.Error("appended xref stream must chain to the previous xref via /Prev")
	}

	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("parse updated PDF: %v", err)
	}
	obj, err := pdf.GetObject(num)
	if err != nil {
		t.Fatalf("GetObject(%d): %v", num, err)
	}
	if !bytes.Contains(obj, []byte("hello world")) {
		t.Errorf("appended stream content not found in object %d: %q", num, obj)
	}
}

// TestClassicalSource_EmitsClassicalTable verifies the pre-existing behavior:
// classical-xref sources keep getting a classical xref table + trailer.
func TestClassicalSource_EmitsClassicalTable(t *testing.T) {
	original := buildPDF(t, false)
	out, num := appendStream(t, original)

	appended := out[len(original):]
	if !bytes.Contains(appended, []byte("\nxref\n")) || !bytes.Contains(appended, []byte("trailer")) {
		t.Error("appended revision must contain a classical xref table and trailer")
	}
	if bytes.Contains(appended, []byte("/Type/XRef")) {
		t.Error("appended revision must not contain a cross-reference stream")
	}

	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("parse updated PDF: %v", err)
	}
	if _, err := pdf.GetObject(num); err != nil {
		t.Fatalf("GetObject(%d): %v", num, err)
	}
}

// TestXRefStreamSource_SizeCoversXRefObject verifies that /Size in the appended
// xref stream accounts for the xref stream object itself (largest objnum + 1).
func TestXRefStreamSource_SizeCoversXRefObject(t *testing.T) {
	original := buildPDF(t, true)
	out, num := appendStream(t, original)

	appended := out[len(original):]
	want := []byte(fmt.Sprintf("/Size %d", num+2)) // num is appended obj, num+1 is the xref stream
	if !bytes.Contains(appended, want) {
		t.Errorf("appended xref stream missing %q:\n%s", want, appended)
	}
}
