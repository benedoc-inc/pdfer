package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/types"
)

const benchmarkDir = "../../tests/resources/benchmark"

func readBenchmarkPDF(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(benchmarkDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("Benchmark PDF not available: %s", path)
	}
	return data
}

func TestBenchmark_Hoare78(t *testing.T) {
	pdfBytes := readBenchmarkPDF(t, "Hoare78.pdf")

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract Hoare78.pdf: %v", err)
	}

	// Page count
	if len(doc.Pages) != 12 {
		t.Errorf("Expected 12 pages, got %d", len(doc.Pages))
	}

	// All 12 pages should have text (verifies indirect content array fix)
	pagesWithText := 0
	for i, page := range doc.Pages {
		if len(page.Text) > 0 {
			pagesWithText++
		} else {
			t.Errorf("Page %d has zero text elements", i+1)
		}
	}
	if pagesWithText < 12 {
		t.Errorf("Only %d of 12 pages have text (expected all 12)", pagesWithText)
	}

	// Font sizes should be > 1 (verifies text matrix fix)
	for i, page := range doc.Pages {
		for _, el := range page.Text {
			if el.FontSize > 0 && el.FontSize <= 1.0 {
				t.Errorf("Page %d: font size %.2f <= 1.0 (text matrix scaling broken): text=%q",
					i+1, el.FontSize, truncate(el.Text, 40))
			}
		}
	}

	// No double spaces in FullText
	fullText := doc.FullText()
	if strings.Contains(fullText, "  ") {
		// Find first occurrence for diagnostics
		idx := strings.Index(fullText, "  ")
		start := idx - 20
		if start < 0 {
			start = 0
		}
		end := idx + 30
		if end > len(fullText) {
			end = len(fullText)
		}
		t.Errorf("FullText contains double spaces near: %q", fullText[start:end])
	}

	// No \x02 control characters (verifies ligature fix)
	if strings.ContainsRune(fullText, '\x02') {
		idx := strings.IndexRune(fullText, '\x02')
		start := idx - 10
		if start < 0 {
			start = 0
		}
		end := idx + 10
		if end > len(fullText) {
			end = len(fullText)
		}
		t.Errorf("FullText contains \\x02 (broken ligature) near: %q", fullText[start:end])
	}

	// TextItems should be generated
	if len(doc.TextItems) == 0 {
		t.Error("Expected TextItems to be generated")
	}

	// Content accuracy: paper title fragment must appear in full text
	if !strings.Contains(fullText, "Sequential Processes") {
		t.Error("FullText should contain 'Sequential Processes' from the paper title")
	}

	// Content accuracy: author name must appear
	if !strings.Contains(fullText, "Hoare") {
		t.Error("FullText should contain author name 'Hoare'")
	}

	// Content accuracy: each page should have substantial text (no nearly-empty pages)
	for i, page := range doc.Pages {
		if len(page.Text) < 50 {
			t.Errorf("Page %d has only %d text elements (expected ≥50)", i+1, len(page.Text))
		}
	}

	// Content accuracy: total text length sanity floor
	if len(fullText) < 50000 {
		t.Errorf("FullText length %d < 50000 (expected substantial content)", len(fullText))
	}

	// Should have some headings but not too many (false positive check)
	headers := doc.SectionHeaders()
	t.Logf("Hoare78: %d pages, %d text items, %d headers, %d chars full text",
		len(doc.Pages), len(doc.TextItems), len(headers), len(fullText))

	if len(headers) == 0 {
		t.Error("Expected at least some section headers")
	}
	if len(headers) > 30 {
		t.Errorf("Too many section headers (%d) — likely false positives", len(headers))
	}

	// Fonts should be detected (paper uses multiple fonts)
	if len(doc.Fonts) == 0 {
		t.Error("Expected fonts to be detected")
	}

	// Label distribution: should have text, list_items, and page_footers
	labelCounts := countLabels(doc)
	if labelCounts[types.TextLabelText] == 0 {
		t.Error("Expected text-labeled items")
	}
	if labelCounts[types.TextLabelListItem] == 0 {
		t.Error("Expected list_item-labeled items (numbered examples in paper)")
	}
	if labelCounts[types.TextLabelPageFooter] == 0 {
		t.Error("Expected page_footer items to be detected")
	}

	// Pictures: each page has an image ref (scanned background)
	if len(doc.Pictures) == 0 {
		t.Error("Expected picture items from page image refs")
	}

	// --- Quality gap diagnostics (logged, not failing) ---

	// GAP: Author/affiliation lines are falsely detected as section headers.
	// "C.A.R. Hoare", "The Queen's University", "Belfast, Northern Ireland"
	// should be author/affiliation, not headers.
	falsePositiveHeaders := 0
	for _, h := range headers {
		lower := strings.ToLower(h.Text)
		if strings.Contains(lower, "hoare") || strings.Contains(lower, "queen") || strings.Contains(lower, "belfast") {
			falsePositiveHeaders++
		}
	}
	if falsePositiveHeaders > 0 {
		t.Logf("QUALITY GAP: %d author/affiliation lines falsely labeled as section_header", falsePositiveHeaders)
	}

	// GAP: Paper's actual numbered sections (1. Introduction, 2. Parallel Commands, etc.)
	// are not detected as headers — only title-page text and a few mid-paper fragments.
	realSectionKeywords := []string{"introduction", "parallel", "coroutines", "monitor", "discussion"}
	realSectionsFound := 0
	for _, h := range headers {
		lower := strings.ToLower(h.Text)
		for _, kw := range realSectionKeywords {
			if strings.Contains(lower, kw) {
				realSectionsFound++
				break
			}
		}
	}
	if realSectionsFound == 0 {
		t.Logf("QUALITY GAP: none of the paper's real numbered sections detected as headers")
	}

	// GAP: First TextItem starts mid-word ("grams, three basic constructs...")
	// — the title "Communicating" and preceding text from the two-column layout
	// may be misordered.
	if len(doc.TextItems) > 0 {
		firstText := doc.TextItems[0].Text
		if strings.HasPrefix(firstText, "grams") {
			t.Logf("QUALITY GAP: first TextItem starts mid-word: %q", truncate(firstText, 60))
		}
	}
}

func TestBenchmark_InParsV2(t *testing.T) {
	pdfBytes := readBenchmarkPDF(t, "inpars_v2.pdf")

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract inpars_v2.pdf: %v", err)
	}

	// Should have pages
	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least 1 page")
	}

	// Font sizes should be > 1
	for i, page := range doc.Pages {
		for _, el := range page.Text {
			if el.FontSize > 0 && el.FontSize <= 1.0 {
				t.Errorf("Page %d: font size %.2f <= 1.0 (text matrix scaling broken): text=%q",
					i+1, el.FontSize, truncate(el.Text, 40))
			}
		}
	}

	// No zero font sizes for elements with actual text
	for i, page := range doc.Pages {
		for _, el := range page.Text {
			if strings.TrimSpace(el.Text) != "" && el.FontSize == 0 {
				t.Errorf("Page %d: FontSize=0.0 for non-empty text %q", i+1, truncate(el.Text, 40))
			}
		}
	}

	// No double spaces
	fullText := doc.FullText()
	if strings.Contains(fullText, "  ") {
		idx := strings.Index(fullText, "  ")
		start := idx - 20
		if start < 0 {
			start = 0
		}
		end := idx + 30
		if end > len(fullText) {
			end = len(fullText)
		}
		t.Errorf("FullText contains double spaces near: %q", fullText[start:end])
	}

	// No \x02 control characters (verifies ligature fix)
	if strings.ContainsRune(fullText, '\x02') {
		idx := strings.IndexRune(fullText, '\x02')
		start := idx - 10
		if start < 0 {
			start = 0
		}
		end := idx + 10
		if end > len(fullText) {
			end = len(fullText)
		}
		t.Errorf("FullText contains \\x02 (broken ligature) near: %q", fullText[start:end])
	}

	// TextItems generated
	if len(doc.TextItems) == 0 {
		t.Error("Expected TextItems to be generated")
	}

	// Content accuracy: paper name must appear
	if !strings.Contains(fullText, "InPars") {
		t.Error("FullText should contain 'InPars'")
	}

	// No \x03 control chars (fl ligature fix)
	if strings.ContainsRune(fullText, '\x03') {
		idx := strings.IndexRune(fullText, '\x03')
		start := idx - 10
		if start < 0 {
			start = 0
		}
		end := idx + 10
		if end > len(fullText) {
			end = len(fullText)
		}
		t.Errorf("FullText contains \\x03 (broken fl ligature) near: %q", fullText[start:end])
	}

	// Content accuracy: specific known sections must exist as headers
	expectedSections := []string{"Abstract", "Introduction", "References"}
	for _, name := range expectedSections {
		if doc.FindSection(name) == nil {
			t.Errorf("Expected to find '%s' section header", name)
		}
	}

	// Should have section headers (verifies >= threshold fix)
	headers := doc.SectionHeaders()
	t.Logf("InParsV2: %d pages, %d text items, %d headers, %d chars full text",
		len(doc.Pages), len(doc.TextItems), len(headers), len(fullText))

	if len(headers) == 0 {
		t.Error("Expected at least some section headers (Abstract, Introduction, etc.)")
	}

	// Exact page count
	if len(doc.Pages) != 4 {
		t.Errorf("Expected 4 pages, got %d", len(doc.Pages))
	}

	// Bookmarks should be detected (paper has 4)
	if len(doc.Bookmarks) == 0 {
		t.Error("Expected bookmarks to be detected")
	}

	// Annotations should exist on pages (hyperlinks in the paper)
	totalAnnotations := 0
	for _, page := range doc.Pages {
		totalAnnotations += len(page.Annotations)
	}
	if totalAnnotations == 0 {
		t.Error("Expected annotations (hyperlinks) on pages")
	}

	// All major academic sections should be detected
	for _, name := range []string{"Methodology", "Results", "Conclusion"} {
		if doc.FindSection(name) == nil {
			t.Errorf("Expected to find '%s' section header", name)
		}
	}

	// --- Quality gap diagnostics (logged, not failing) ---

	// GAP: arXiv metadata line falsely detected as L1 (highest-level) header
	for _, h := range headers {
		if strings.Contains(h.Text, "arXiv") {
			t.Logf("QUALITY GAP: arXiv metadata line detected as L%d header: %q", h.Level, truncate(h.Text, 60))
		}
	}

	// GAP: Table data row falsely detected as header
	for _, h := range headers {
		if strings.HasPrefix(h.Text, "FEVER") {
			t.Logf("QUALITY GAP: table data row detected as L%d header: %q", h.Level, truncate(h.Text, 60))
		}
	}

	// GAP: Missing word spaces in title and section names
	for _, h := range headers {
		if strings.Contains(h.Text, "LargeLanguage") || strings.Contains(h.Text, "Introductionand") ||
			strings.Contains(h.Text, "ModelsasEf") || strings.Contains(h.Text, "GeneratorsforInformation") {
			t.Logf("QUALITY GAP: missing word spaces in header: %q", truncate(h.Text, 70))
		}
	}

	// GAP: Title split across two separate headers instead of one
	titleParts := 0
	for _, h := range headers {
		if strings.Contains(h.Text, "InPars") || strings.Contains(h.Text, "Retrieval") {
			if h.Level == 2 {
				titleParts++
			}
		}
	}
	if titleParts > 1 {
		t.Logf("QUALITY GAP: paper title split across %d separate L2 headers", titleParts)
	}

	// GAP: 0 fonts detected despite the paper using multiple fonts
	if len(doc.Fonts) == 0 {
		t.Logf("QUALITY GAP: no fonts detected in InParsV2 (CIDFont extraction not yet supported)")
	}
}

func TestBenchmark_InParsV2_Headers(t *testing.T) {
	pdfBytes := readBenchmarkPDF(t, "inpars_v2.pdf")

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract inpars_v2.pdf: %v", err)
	}

	headers := doc.SectionHeaders()
	if len(headers) == 0 {
		t.Fatal("No section headers found")
	}

	// "Abstract" should be labeled section_header with Level 3
	abstractHeader := doc.FindSection("Abstract")
	if abstractHeader == nil {
		t.Error("Expected 'Abstract' to be detected as section header")
	} else if abstractHeader.Level != 3 && abstractHeader.Level != 4 {
		t.Logf("Abstract heading level: %d (expected 3 or 4)", abstractHeader.Level)
	}

	// "References" should be detected
	if doc.FindSection("References") == nil {
		t.Error("Expected 'References' to be detected as section header")
	}

	// Section numbering: look for "Introduction" (may have "1 " prefix)
	foundIntro := false
	for _, h := range headers {
		lower := strings.ToLower(h.Text)
		if strings.Contains(lower, "introduction") {
			foundIntro = true
			break
		}
	}
	if !foundIntro {
		t.Error("Expected an Introduction-related header")
	}
}

func TestBenchmark_Hoare78_Structure(t *testing.T) {
	pdfBytes := readBenchmarkPDF(t, "Hoare78.pdf")

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract Hoare78.pdf: %v", err)
	}

	// First TextItem on page 1 should be meaningful (not garbage)
	page1Items := doc.PageTextItems(1)
	if len(page1Items) == 0 {
		t.Fatal("No text items on page 1")
	}
	first := page1Items[0]
	if len(strings.TrimSpace(first.Text)) < 2 {
		t.Errorf("First text item on page 1 is too short: %q", first.Text)
	}

	// Headers should be spread across pages (not all on page 1)
	headers := doc.SectionHeaders()
	if len(headers) > 1 {
		headerPages := make(map[int]bool)
		for _, h := range headers {
			if h.BBox != nil {
				headerPages[h.BBox.Page] = true
			}
		}
		if len(headerPages) < 2 {
			t.Errorf("All %d headers are on a single page — expected spread across pages", len(headers))
		}
	}

	// TextBetweenHeaders should return non-empty text for at least one pair of
	// non-adjacent headers (some consecutive headers on the title page may have
	// nothing between them)
	if len(headers) >= 2 {
		foundNonEmpty := false
		for i := 0; i < len(headers)-1 && !foundNonEmpty; i++ {
			text := doc.TextBetweenHeaders(headers[i].Text, headers[i+1].Text)
			if text != "" {
				foundNonEmpty = true
			}
		}
		if !foundNonEmpty {
			t.Error("TextBetweenHeaders returned empty for all consecutive header pairs")
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func countLabels(doc *types.ContentDocument) map[types.TextLabel]int {
	counts := make(map[types.TextLabel]int)
	for _, item := range doc.TextItems {
		counts[item.Label]++
	}
	return counts
}
