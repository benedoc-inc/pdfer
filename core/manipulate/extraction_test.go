package manipulate

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

const twoPageFixture = "../../tests/resources/K141167_summary_1.pdf"

// TestExtractPagesDropsUnselectedContent is the regression test for the
// orphan-retention bug: extraction used to follow /Parent from a selected page
// into its /Pages node, whose /Kids reach every other page in the document, so
// the walk collected the WHOLE file. Removing the page objects afterwards left
// every unselected page's fonts, images and content streams in the output —
// orphaned but still written — and an "extracted" page came out the size of
// its source.
//
// Measured on this fixture: page 1 of 2 was 94.8% of the source before the fix
// and 47.2% after. The threshold sits between those, well clear of both.
func TestExtractPagesDropsUnselectedContent(t *testing.T) {
	source, err := os.ReadFile(twoPageFixture)
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := ExtractPages(source, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}

	share := float64(len(out)) / float64(len(source))
	if share > 0.75 {
		t.Errorf("extracted 1 of 2 pages but kept %.1f%% of the source (%d of %d bytes) — "+
			"unselected pages' content is being carried over",
			100*share, len(out), len(source))
	}

	m, err := NewPDFManipulator(out, nil, false)
	if err != nil {
		t.Fatalf("extracted PDF does not parse: %v", err)
	}
	pages, err := m.getAllPageObjectNumbers()
	if err != nil {
		t.Fatalf("extracted PDF page tree: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("extracted PDF reports %d pages, want 1", len(pages))
	}
}

// buildPDF writes a small valid PDF from object bodies (1-indexed), with a
// correct xref table. Used to construct the exact page-tree shapes the
// extraction edge cases need, which no fixture happens to have.
func buildPDF(objects []string, rootObjNum int) []byte {
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

// TestExtractPagesMaterializesInheritedAttributes pins that attributes a page
// INHERITS from its /Pages node survive extraction.
//
// The page tree is discarded during extraction, so anything a page was
// inheriting (/MediaBox, /Resources, /CropBox, /Rotate — ISO 32000-1 §7.7.3.4)
// has to be copied onto the page itself. Otherwise a page whose size lived on
// the parent comes out with no /MediaBox at all, which readers render as a
// default-sized or blank page.
func TestExtractPagesMaterializesInheritedAttributes(t *testing.T) {
	// MediaBox and Resources live ONLY on the Pages node; neither page has them.
	src := buildPDF([]string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R 4 0 R]/Count 2/MediaBox[0 0 300 400]/Resources 5 0 R>>",
		"<</Type/Page/Parent 2 0 R/Contents 6 0 R>>",
		"<</Type/Page/Parent 2 0 R/Contents 7 0 R>>",
		"<</Font<</F1 8 0 R>>>>",
		"<</Length 8>>\nstream\nBT ET \nendstream",
		"<</Length 8>>\nstream\nBT ET \nendstream",
		"<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>",
	}, 1)

	out, err := ExtractPages(src, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "/MediaBox[0 0 300 400]") && !strings.Contains(got, "/MediaBox [0 0 300 400]") {
		t.Errorf("inherited /MediaBox was lost — the extracted page has no size:\n%s", got)
	}

	// A bare `strings.Contains(got, "/Resources")` is satisfied by broken output
	// like "/Resources 5" (a dangling integer) or "/Resources null", so it can't
	// tell a real materialization from a lost one. Parse the extract and verify
	// the inherited /Resources is a valid indirect reference whose target — and
	// that target's font — actually survived into the output.
	m, err := NewPDFManipulator(out, nil, false)
	if err != nil {
		t.Fatalf("extracted PDF does not parse: %v", err)
	}
	pages, err := m.getAllPageObjectNumbers()
	if err != nil || len(pages) != 1 {
		t.Fatalf("extracted page tree: pages=%v err=%v", pages, err)
	}
	page := string(m.objects[pages[0]])

	res := dictValue(page, "/Resources")
	if !refAtStart.MatchString(res) {
		t.Fatalf("inherited /Resources materialized as %q, not a valid indirect "+
			"reference — the resource object was lost:\n%s", res, page)
	}
	var resNum int
	if _, err := fmt.Sscanf(res, "%d", &resNum); err != nil {
		t.Fatalf("could not parse /Resources reference %q: %v", res, err)
	}
	resObj, ok := m.objects[resNum]
	if !ok {
		t.Fatalf("/Resources points at object %d, absent from the extract:\n%s", resNum, page)
	}
	if !strings.Contains(string(resObj), "/Font") {
		t.Errorf("resources object %d carries no /Font:\n%s", resNum, string(resObj))
	}
	if !strings.Contains(got, "/BaseFont/Helvetica") {
		t.Errorf("the inherited resources' font was not carried into the extract:\n%s", got)
	}

	// The page must carry exactly one /Parent, pointing at the new /Pages node —
	// not a duplicate, not a reference remapped onto some unrelated object, and
	// materializing inherited attributes must not have clobbered /Contents.
	if n := strings.Count(page, "/Parent"); n != 1 {
		t.Errorf("extracted page has %d /Parent entries, want 1:\n%s", n, page)
	}
	parent := dictValue(page, "/Parent")
	var parentNum int
	if _, err := fmt.Sscanf(parent, "%d", &parentNum); err != nil {
		t.Fatalf("could not parse /Parent reference %q: %v", parent, err)
	}
	if !strings.Contains(string(m.objects[parentNum]), "/Type/Pages") {
		t.Errorf("/Parent points at object %d, which is not the /Pages node:\n%s",
			parentNum, string(m.objects[parentNum]))
	}
	if !strings.Contains(page, "/Contents") {
		t.Errorf("materializing inherited attributes clobbered /Contents:\n%s", page)
	}
}

// TestExtractPagesNullsReferencesToDroppedObjects pins that a reference to an
// object which was NOT carried over becomes null rather than keeping its
// original number.
//
// Extraction renumbers objects from 1, so a reference left at its source
// number does not dangle harmlessly — it points at whatever unrelated object
// now holds that number. A link annotation targeting a dropped page would
// resolve, in the extracted file, to a font or an image.
func TestExtractPagesNullsReferencesToDroppedObjects(t *testing.T) {
	// Page 1 carries an annotation whose /Dest points at page 2 (object 4),
	// which extraction drops.
	src := buildPDF([]string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R 4 0 R]/Count 2/MediaBox[0 0 300 400]>>",
		"<</Type/Page/Parent 2 0 R/Contents 5 0 R/Annots[6 0 R]>>",
		"<</Type/Page/Parent 2 0 R/Contents 7 0 R>>",
		"<</Length 8>>\nstream\nBT ET \nendstream",
		"<</Type/Annot/Subtype/Link/Rect[0 0 10 10]/Dest[4 0 R /Fit]>>",
		"<</Length 8>>\nstream\nBT ET \nendstream",
	}, 1)

	out, err := ExtractPages(src, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}
	if !strings.Contains(string(out), "/Dest[null") {
		t.Errorf("reference to the dropped page was not nulled — it now points at "+
			"whatever object took that number:\n%s", string(out))
	}
}

// TestExtractPagesDoesNotRewriteStreamBodies pins that reference remapping stays
// inside object dictionaries and never touches stream bodies. A stream body can
// contain bytes that look like an indirect reference ("999 0 R" here); rewriting
// such a false match to "null" would both corrupt the content and change its
// length so it no longer matches the declared /Length.
func TestExtractPagesDoesNotRewriteStreamBodies(t *testing.T) {
	src := buildPDF([]string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R]/Count 1/MediaBox[0 0 300 400]>>",
		"<</Type/Page/Parent 2 0 R/Contents 4 0 R>>",
		"<</Length 18>>\nstream\nBT (999 0 R) Tj ET\nendstream",
	}, 1)

	out, err := ExtractPages(src, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "BT (999 0 R) Tj ET") {
		t.Errorf("stream body was rewritten — a reference-like sequence inside it "+
			"was mangled during remapping:\n%s", got)
	}
}

// The dropped page's own content must not survive either — the size assertion
// above catches this in aggregate, this catches it precisely.
func TestExtractPagesDropsUnselectedPageContent(t *testing.T) {
	src := buildPDF([]string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R 4 0 R]/Count 2/MediaBox[0 0 300 400]>>",
		"<</Type/Page/Parent 2 0 R/Contents 5 0 R>>",
		"<</Type/Page/Parent 2 0 R/Contents 6 0 R>>",
		"<</Length 26>>\nstream\nBT (KEEP THIS PAGE) Tj ET\nendstream",
		"<</Length 26>>\nstream\nBT (DROP THIS PAGE) Tj ET\nendstream",
	}, 1)

	out, err := ExtractPages(src, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "KEEP THIS PAGE") {
		t.Error("selected page's content stream is missing")
	}
	if strings.Contains(got, "DROP THIS PAGE") {
		t.Error("unselected page's content stream was carried into the extract")
	}
}
