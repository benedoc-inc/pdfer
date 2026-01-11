package parse

import (
	"bytes"
	"fmt"
	"testing"
)

// createSimplePDF creates a minimal valid PDF for testing
func createSimplePDF() []byte {
	var buf bytes.Buffer

	buf.WriteString("%PDF-1.7\n")
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A})

	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")

	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n")

	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>\nendobj\n")

	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))

	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 4/Root 1 0 R>>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

// createIncrementalPDF creates a PDF with multiple revisions
func createIncrementalPDF() []byte {
	// Start with a basic PDF
	var buf bytes.Buffer

	// First revision
	buf.WriteString("%PDF-1.7\n")
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A}) // Binary marker

	// Object 1: Catalog
	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")

	// Object 2: Pages
	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n")

	// Object 3: Page
	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R>>\nendobj\n")

	// Object 4: Content stream (first version)
	obj4Offset := buf.Len()
	buf.WriteString("4 0 obj\n<</Length 29>>\nstream\nBT /F1 12 Tf (Hello) Tj ET\nendstream\nendobj\n")

	// First xref
	xref1Offset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("0 5\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(formatXRefEntry(obj1Offset))
	buf.WriteString(formatXRefEntry(obj2Offset))
	buf.WriteString(formatXRefEntry(obj3Offset))
	buf.WriteString(formatXRefEntry(obj4Offset))

	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 5/Root 1 0 R>>\n")
	buf.WriteString("startxref\n")
	buf.WriteString(formatInt(xref1Offset) + "\n")
	buf.WriteString("%%EOF\n")

	// Second revision (incremental update)
	// Update object 4 with new content
	obj4NewOffset := buf.Len()
	buf.WriteString("4 0 obj\n<</Length 35>>\nstream\nBT /F1 12 Tf (Hello World!) Tj ET\nendstream\nendobj\n")

	// Add new object 5
	obj5Offset := buf.Len()
	buf.WriteString("5 0 obj\n<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>\nendobj\n")

	// Second xref (incremental)
	xref2Offset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("4 2\n") // Objects 4 and 5
	buf.WriteString(formatXRefEntry(obj4NewOffset))
	buf.WriteString(formatXRefEntry(obj5Offset))

	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 6/Root 1 0 R/Prev " + formatInt(xref1Offset) + ">>\n")
	buf.WriteString("startxref\n")
	buf.WriteString(formatInt(xref2Offset) + "\n")
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func formatXRefEntry(offset int) string {
	return fmt.Sprintf("%010d 00000 n \n", offset)
}

func formatInt(n int) string {
	return fmt.Sprintf("%d", n)
}

func TestFindAllEOFMarkers(t *testing.T) {
	pdf := createIncrementalPDF()
	markers := FindAllEOFMarkers(pdf)

	if len(markers) != 2 {
		t.Errorf("Expected 2 %%EOF markers, found %d", len(markers))
	}

	t.Logf("Found %%EOF markers at: %v", markers)
}

func TestCountRevisions(t *testing.T) {
	pdf := createIncrementalPDF()
	count := CountRevisions(pdf)

	if count != 2 {
		t.Errorf("Expected 2 revisions, found %d", count)
	}
}

func TestIncrementalParser_Parse(t *testing.T) {
	pdf := createIncrementalPDF()

	parser := newIncrementalParser(pdf, true)
	err := parser.parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	sections := parser.getSections()
	if len(sections) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(sections))
	}

	// Check merged objects
	objMap := parser.getObjectMap()

	// Should have objects 1-5
	for i := 1; i <= 5; i++ {
		if _, ok := objMap[i]; !ok {
			t.Errorf("Object %d not found in merged map", i)
		}
	}

	// Object 4 should point to the updated version (from second revision)
	// The second revision's offset should be greater than first
	if len(sections) >= 2 {
		oldOffset := sections[0].Objects[4]
		newOffset := sections[1].Objects[4]

		if newOffset <= oldOffset {
			t.Errorf("Object 4 should be updated: old=%d, new=%d", oldOffset, newOffset)
		}

		// Merged map should have the newer offset
		if objMap[4] != newOffset {
			t.Errorf("Merged object 4 should be %d, got %d", newOffset, objMap[4])
		}
	}

	t.Logf("Parsed %d sections with %d merged objects", len(sections), len(objMap))
}

func TestParseWithIncrementalUpdates(t *testing.T) {
	pdf := createIncrementalPDF()

	result, err := parseWithIncrementalUpdates(pdf, false)
	if err != nil {
		t.Fatalf("parseWithIncrementalUpdates failed: %v", err)
	}

	// Should have at least 5 objects
	if len(result.Objects) < 5 {
		t.Errorf("Expected at least 5 objects, got %d", len(result.Objects))
	}

	t.Logf("Parsed %d objects, %d in streams", len(result.Objects), len(result.ObjectStreams))
}

func TestExtractRevision(t *testing.T) {
	pdf := createIncrementalPDF()

	// Extract first revision
	rev1, err := ExtractRevision(pdf, 1)
	if err != nil {
		t.Fatalf("ExtractRevision(1) failed: %v", err)
	}

	// First revision should be smaller than full PDF
	if len(rev1) >= len(pdf) {
		t.Errorf("First revision (%d) should be smaller than full PDF (%d)", len(rev1), len(pdf))
	}

	// First revision should end with %%EOF
	if !bytes.Contains(rev1, []byte("%%EOF")) {
		t.Error("First revision should contain EOF marker")
	}

	// First revision should only have 1 %%EOF
	if CountRevisions(rev1) != 1 {
		t.Errorf("First revision should have 1 EOF marker, got %d", CountRevisions(rev1))
	}

	// Extract second revision (full PDF)
	rev2, err := ExtractRevision(pdf, 2)
	if err != nil {
		t.Fatalf("ExtractRevision(2) failed: %v", err)
	}

	if CountRevisions(rev2) != 2 {
		t.Errorf("Second revision should have 2 %%EOF markers, got %d", CountRevisions(rev2))
	}

	t.Logf("Revision 1: %d bytes, Revision 2: %d bytes", len(rev1), len(rev2))
}

func TestIncrementalParser_SingleRevision(t *testing.T) {
	// Test with a simple single-revision PDF
	pdf := createSimplePDF()

	parser := newIncrementalParser(pdf, false)
	err := parser.parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	sections := parser.getSections()
	if len(sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(sections))
	}

	objMap := parser.getObjectMap()
	if len(objMap) == 0 {
		t.Error("Should have parsed some objects")
	}

	t.Logf("Single revision: %d objects", len(objMap))
}

func TestPrevChain(t *testing.T) {
	pdf := createIncrementalPDF()

	parser := newIncrementalParser(pdf, false)
	err := parser.parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	sections := parser.getSections()

	// First section (oldest) should have Prev = 0
	if sections[0].Prev != 0 {
		t.Errorf("First section Prev should be 0, got %d", sections[0].Prev)
	}

	// Second section should have Prev pointing to first xref
	if sections[1].Prev == 0 {
		t.Error("Second section should have non-zero Prev")
	}

	// Verify Prev points to a valid location
	if sections[1].Prev != sections[0].StartXRef {
		t.Errorf("Second section Prev (%d) should equal first section StartXRef (%d)",
			sections[1].Prev, sections[0].StartXRef)
	}
}

func TestGetRevisionBoundaries(t *testing.T) {
	pdf := createIncrementalPDF()

	boundaries := GetRevisionBoundaries(pdf)

	if len(boundaries) != 2 {
		t.Errorf("Expected 2 boundaries, got %d", len(boundaries))
	}

	// Each boundary should be after a %%EOF
	for i, b := range boundaries {
		if b > len(pdf) {
			t.Errorf("Boundary %d (%d) exceeds PDF length (%d)", i, b, len(pdf))
		}
	}

	// Boundaries should be in order
	for i := 1; i < len(boundaries); i++ {
		if boundaries[i] <= boundaries[i-1] {
			t.Errorf("Boundaries should be increasing: %d <= %d", boundaries[i], boundaries[i-1])
		}
	}

	t.Logf("Revision boundaries: %v", boundaries)
}
