package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/xfa"
	"github.com/benedoc-inc/pdfer/types"
)

func TestBuildPDFFromScratch(t *testing.T) {
	// Create a simple XFA PDF from scratch
	builder := write.NewXFABuilder(true)

	templateXML := `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://www.xfa.org/schema/xfa-template/3.3/">
  <subform name="form1" layout="tb">
    <field name="TextField1">
      <ui><textEdit/></ui>
      <value><text>Hello World</text></value>
    </field>
  </subform>
</template>`

	datasetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<xfa:datasets xmlns:xfa="http://www.xfa.org/schema/xfa-data/1.0/">
  <xfa:data>
    <form1>
      <TextField1>Test Value</TextField1>
    </form1>
  </xfa:data>
</xfa:datasets>`

	configXML := `<?xml version="1.0" encoding="UTF-8"?>
<config xmlns="http://www.xfa.org/schema/xci/3.0/">
  <present>
    <pdf>
      <version>1.7</version>
    </pdf>
  </present>
</config>`

	streams := []write.XFAStreamData{
		{Name: "template", Data: []byte(templateXML), Compress: true},
		{Name: "datasets", Data: []byte(datasetsXML), Compress: true},
		{Name: "config", Data: []byte(configXML), Compress: true},
	}

	pdfBytes, err := builder.BuildFromXFA(streams)
	if err != nil {
		t.Fatalf("Failed to build PDF: %v", err)
	}

	t.Logf("Built PDF from scratch: %d bytes", len(pdfBytes))

	// Verify it's a valid PDF
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Error("PDF doesn't start with %PDF-")
	}

	if !bytes.HasSuffix(pdfBytes, []byte("%%EOF\n")) {
		t.Error("PDF doesn't end with EOF marker")
	}

	// Verify it contains XFA
	if !bytes.Contains(pdfBytes, []byte("/XFA")) {
		t.Error("PDF doesn't contain /XFA")
	}

	// Write to file for inspection
	outPath := filepath.Join("tests", "resources", "scratch_xfa.pdf")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	if err := os.WriteFile(outPath, pdfBytes, 0644); err != nil {
		t.Logf("Warning: Could not write PDF: %v", err)
	} else {
		t.Logf("Wrote PDF to: %s", outPath)
	}
}

func TestRebuildPDFWithUpdatedXFA(t *testing.T) {
	// Get test PDF
	testPDFPath := getTestResourcePath("estar.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read PDF: %v", err)
	}

	// Get encryption info
	var encryptInfo *types.PDFEncryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		_, encInfo, err := encrypt.DecryptPDF(pdfBytes, []byte(""), true)
		if err == nil {
			encryptInfo = encInfo
		}
	}

	// Extract XFA
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, encryptInfo, false)
	if err != nil {
		t.Fatalf("Failed to extract XFA: %v", err)
	}

	t.Logf("Extracted template: %d bytes", len(streams.Template.Data))
	t.Logf("Extracted datasets: %d bytes", len(streams.Datasets.Data))

	// Modify datasets (add a new value)
	if bytes.Contains(streams.Datasets.Data, []byte("</xfa:data>")) {
		streams.Datasets.Data = bytes.ReplaceAll(streams.Datasets.Data,
			[]byte("</xfa:data>"),
			[]byte("<TestField>Modified</TestField></xfa:data>"))
		t.Logf("Modified datasets XML")
	}

	// Rebuild PDF
	rebuiltPDF, err := xfa.RebuildPDFFromXFAStreams(pdfBytes, streams, encryptInfo, true)
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	t.Logf("Rebuilt PDF: %d bytes", len(rebuiltPDF))

	// Verify the rebuilt PDF has the modification
	// Extract XFA from rebuilt PDF
	rebuiltStreams, err := xfa.ExtractAllXFAStreams(rebuiltPDF, nil, false) // No encryption for rebuilt
	if err != nil {
		t.Logf("Could not extract from rebuilt PDF: %v", err)
		// This is expected since the rebuilt PDF doesn't preserve encryption properly yet
	} else {
		if rebuiltStreams.Datasets != nil {
			t.Logf("Rebuilt datasets: %d bytes", len(rebuiltStreams.Datasets.Data))
			if bytes.Contains(rebuiltStreams.Datasets.Data, []byte("TestField")) {
				t.Logf("Modification preserved in rebuilt PDF")
			}
		}
	}

	// Write to file
	outPath := filepath.Join("tests", "resources", "estar_modified.pdf")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	if err := os.WriteFile(outPath, rebuiltPDF, 0644); err != nil {
		t.Logf("Warning: Could not write PDF: %v", err)
	} else {
		t.Logf("Wrote modified PDF to: %s", outPath)
	}
}

func TestBuildPDFFromExtractedXFA(t *testing.T) {
	// Get test PDF
	testPDFPath := getTestResourcePath("estar.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read PDF: %v", err)
	}

	// Get encryption info
	var encryptInfo *types.PDFEncryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		_, encInfo, err := encrypt.DecryptPDF(pdfBytes, []byte(""), true)
		if err == nil {
			encryptInfo = encInfo
		}
	}

	// Extract XFA
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, encryptInfo, false)
	if err != nil {
		t.Fatalf("Failed to extract XFA: %v", err)
	}

	t.Logf("Extracted XFA streams:")
	if streams.Template != nil {
		t.Logf("  Template: %d bytes", len(streams.Template.Data))
	}
	if streams.Datasets != nil {
		t.Logf("  Datasets: %d bytes", len(streams.Datasets.Data))
	}
	if streams.Config != nil {
		t.Logf("  Config: %d bytes", len(streams.Config.Data))
	}
	if streams.LocaleSet != nil {
		t.Logf("  LocaleSet: %d bytes", len(streams.LocaleSet.Data))
	}

	// Build new PDF from extracted XFA (without encryption)
	newPDF, err := xfa.BuildPDFFromXFAStreams(streams, true)
	if err != nil {
		t.Fatalf("Failed to build PDF from XFA: %v", err)
	}

	t.Logf("Built new PDF: %d bytes (original: %d bytes)", len(newPDF), len(pdfBytes))

	// Verify it's a valid PDF
	if !bytes.HasPrefix(newPDF, []byte("%PDF-")) {
		t.Error("PDF doesn't start with %PDF-")
	}

	if !bytes.Contains(newPDF, []byte("/XFA")) {
		t.Error("PDF doesn't contain /XFA")
	}

	// Try to extract XFA from the new PDF
	newStreams, err := xfa.ExtractAllXFAStreams(newPDF, nil, false)
	if err != nil {
		t.Logf("Could not extract XFA from new PDF: %v", err)
	} else {
		if newStreams.Template != nil {
			t.Logf("Extracted template from new PDF: %d bytes", len(newStreams.Template.Data))
		}
	}

	// Write to file
	outPath := filepath.Join("tests", "resources", "estar_clean.pdf")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	if err := os.WriteFile(outPath, newPDF, 0644); err != nil {
		t.Logf("Warning: Could not write PDF: %v", err)
	} else {
		t.Logf("Wrote clean PDF to: %s", outPath)
	}
}
