package acroform

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// buildIDTestForm builds a minimal single text-field AcroForm PDF.
func buildIDTestForm(t *testing.T) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	builder.FinalizePage(page)

	fb := NewFormBuilder(builder)
	fb.AddTextField("fullname", []float64{72, 700, 300, 720}, 0)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm: %v", err)
	}
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	// The builder emits no /ID; inject one into the classic trailer dict. This
	// sits after the xref table, so xref offsets and startxref are unaffected.
	const id = "0123456789ABCDEF0123456789ABCDEF"
	marker := []byte("trailer\n<<")
	i := bytes.LastIndex(b, marker)
	if i < 0 {
		t.Fatal("builder output has no classic trailer to inject /ID into")
	}
	at := i + len(marker)
	idEntry := []byte("/ID [<" + id + "><" + id + ">]")
	out := make([]byte, 0, len(b)+len(idEntry))
	out = append(out, b[:at]...)
	out = append(out, idEntry...)
	out = append(out, b[at:]...)
	return out
}

// TestFillIncremental_CarriesFileID verifies that filling a field via the shared
// incremental writer carries the document /ID into the new trailer, rather than
// dropping it as the previous hand-rolled trailer code did.
func TestFillIncremental_CarriesFileID(t *testing.T) {
	base := buildIDTestForm(t)

	origID := extractTrailerIDForTest(base)
	if len(origID) == 0 {
		t.Skip("base PDF has no /ID to preserve")
	}

	const value = "Ada Lovelace"
	out, err := fillIncremental(base, types.FormData{"fullname": value}, nil, false)
	if err != nil {
		t.Fatalf("fillIncremental: %v", err)
	}

	lastTrailer := out[bytes.LastIndex(out, []byte("trailer")):]
	if !bytes.Contains(lastTrailer, []byte("/ID")) {
		t.Fatal("new trailer dropped /ID")
	}
	if !bytes.Contains(lastTrailer, origID) {
		t.Errorf("new trailer /ID %q does not carry the original %q", lastTrailer, origID)
	}

	// The fill must still round-trip.
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	form, err := ParseAcroForm(out, nil, false)
	if err != nil {
		t.Fatalf("re-parse form: %v", err)
	}
	f := form.FindFieldByName("fullname")
	if f == nil {
		t.Fatal("fullname field not found after fill")
	}
	body, err := pdf.GetObjectContent(f.ObjectNum)
	if err != nil {
		t.Fatalf("read field object %d: %v", f.ObjectNum, err)
	}
	if !bytes.Contains(body, []byte(value)) {
		t.Errorf("filled value not present in field object; got %q", body)
	}
}

// extractTrailerIDForTest returns the raw /ID array bytes from the file, or nil.
func extractTrailerIDForTest(pdfBytes []byte) []byte {
	idx := bytes.LastIndex(pdfBytes, []byte("/ID"))
	if idx < 0 {
		return nil
	}
	open := bytes.IndexByte(pdfBytes[idx:], '[')
	if open < 0 {
		return nil
	}
	open += idx
	closeIdx := bytes.IndexByte(pdfBytes[open:], ']')
	if closeIdx < 0 {
		return nil
	}
	return pdfBytes[open : open+closeIdx+1]
}
