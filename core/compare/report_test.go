package compare

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestGenerateReport(t *testing.T) {
	// Create two different PDFs
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Version 1")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Version 2")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	report := GenerateReport(result)
	if !strings.Contains(report, "DIFFERENT") {
		t.Error("Report should indicate PDFs are different")
	}

	if !strings.Contains(report, "Page") {
		t.Error("Report should contain page information")
	}

	t.Logf("Report:\n%s", report)
}

func TestGenerateJSONReport(t *testing.T) {
	// Create two different PDFs
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Version 1")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Version 2")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	jsonReport, err := GenerateJSONReport(result)
	if err != nil {
		t.Fatalf("Failed to generate JSON report: %v", err)
	}

	if !strings.Contains(jsonReport, "differences") {
		t.Error("JSON report should contain 'differences'")
	}

	if !strings.Contains(jsonReport, "identical") {
		t.Error("JSON report should contain 'identical'")
	}
}
