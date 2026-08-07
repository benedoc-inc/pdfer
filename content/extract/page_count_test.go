package extract

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
)

// TestPageCountIsPagesNotObjects is the regression test for
// DocumentMetadata.PageCount reporting the OBJECT count.
//
// ExtractMetadata initialized PageCount to pdf.ObjectCount() with a "will be
// updated when we parse pages" comment that was never honoured, so the value
// was off by whatever ratio of objects to pages a file happened to have. The
// fixture below has 73 objects and 7 pages, and reported 73.
func TestPageCountIsPagesNotObjects(t *testing.T) {
	for _, tc := range []struct {
		path      string
		wantPages int
	}{
		{"../../tests/resources/objstm_xrefstream.pdf", 7},
		{"../../tests/resources/K141167_summary_1.pdf", 2},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Skipf("fixture unavailable: %v", err)
		}
		pdf, err := parse.OpenWithOptions(data, parse.ParseOptions{})
		if err != nil {
			t.Fatalf("%s: open: %v", tc.path, err)
		}
		md, err := ExtractMetadata(data, pdf, false)
		if err != nil {
			t.Fatalf("%s: metadata: %v", tc.path, err)
		}
		if md.PageCount != tc.wantPages {
			t.Errorf("%s: PageCount = %d, want %d (object count is %d)",
				tc.path, md.PageCount, tc.wantPages, pdf.ObjectCount())
		}
	}
}

// countPages must fall back to walking /Kids when the root /Pages node carries
// no usable /Count.
func TestCountPagesFallsBackToLeafWalk(t *testing.T) {
	raw := buildTestPDF([]string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R 4 0 R]>>", // deliberately no /Count
		"<</Type/Page/Parent 2 0 R>>",
		"<</Type/Page/Parent 2 0 R>>",
	}, 1)
	pdf, err := parse.OpenWithOptions(raw, parse.ParseOptions{})
	if err != nil {
		t.Skipf("synthetic PDF not parseable: %v", err)
	}
	if got := countPages(pdf, false); got != 2 {
		t.Errorf("countPages = %d, want 2 from the /Kids walk", got)
	}
}

// buildTestPDF writes a small valid PDF from object bodies (1-indexed) with a
// correct xref table, so the parser can resolve objects by offset.
func buildTestPDF(objects []string, rootObjNum int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, body := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<</Size %d/Root %d 0 R>>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, rootObjNum, xref)
	return buf.Bytes()
}
