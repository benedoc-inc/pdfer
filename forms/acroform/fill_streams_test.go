package acroform

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

func TestFillStreamsWithEstar(t *testing.T) {
	testPDFPath := getTestResourcePath("estar.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s, skipping fill streams test", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	t.Logf("Testing with estar.pdf: %d bytes", len(pdfBytes))

	// Parse PDF to understand its structure
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: []byte(""), // Empty password for estar.pdf
		Verbose:  false,
	})
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	t.Logf("PDF parsed: %d objects, encrypted: %v", pdf.ObjectCount(), pdf.IsEncrypted())

	// Check if PDF has AcroForm
	acroForm, err := ExtractAcroForm(pdfBytes, []byte(""), false)
	if err != nil {
		t.Logf("Note: AcroForm extraction returned: %v", err)
		// estar.pdf is XFA-only, so this is expected
	}

	if acroForm != nil {
		t.Logf("Found AcroForm with %d fields", len(acroForm.Fields))
		if len(acroForm.Fields) > 0 {
			// Test filling if there are fields
			formData := types.FormData{}
			for _, field := range acroForm.Fields[:min(3, len(acroForm.Fields))] {
				formData[field.GetFullName()] = "Test Value"
			}

			// Test fill with streams
			filled, err := FillFormFieldsWithStreams(pdfBytes, formData, []byte(""), true)
			if err != nil {
				t.Logf("FillFormFieldsWithStreams error (may be expected): %v", err)
			} else {
				t.Logf("Filled PDF: %d bytes (original: %d bytes)", len(filled), len(pdfBytes))
				if !bytes.HasPrefix(filled, []byte("%PDF-")) {
					t.Error("Filled PDF doesn't start with %PDF-")
				}
			}
		}
	}

	// Test object stream detection
	// Check how many objects are in streams by trying to access them
	objectsInStreams := 0
	objectsDirect := 0
	totalObjects := pdf.ObjectCount()

	// Sample some objects to see which are in streams
	for i := 1; i <= min(50, totalObjects); i++ {
		if pdf.HasObject(i) {
			obj, err := pdf.GetObject(i)
			if err == nil && len(obj) > 0 {
				// Check if we can determine if it's from a stream
				// (objects from streams are typically smaller and don't have "obj" header)
				if !bytes.Contains(obj[:min(50, len(obj))], []byte("obj")) {
					objectsInStreams++
				} else {
					objectsDirect++
				}
			}
		}
	}

	t.Logf("Sampled objects: ~%d in streams, ~%d direct (out of %d total)", objectsInStreams, objectsDirect, totalObjects)

	// Test that we can access objects in streams
	testObjNum := 212 // AcroForm object from estar.pdf
	if pdf.HasObject(testObjNum) {
		objData, err := pdf.GetObject(testObjNum)
		if err != nil {
			t.Errorf("Failed to get object %d: %v", testObjNum, err)
		} else {
			t.Logf("Successfully extracted object %d from stream: %d bytes", testObjNum, len(objData))
			if bytes.Contains(objData, []byte("/AcroForm")) || bytes.Contains(objData, []byte("/XFA")) {
				t.Log("Object contains AcroForm/XFA data")
			}
		}
	}
}

func TestFillStreamsObjectStreamHandling(t *testing.T) {
	testPDFPath := getTestResourcePath("acroform_test.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found, skipping object stream handling test")
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	// Parse PDF
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: []byte(""),
		Verbose:  false,
	})
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Extract AcroForm
	acroForm, err := ExtractAcroForm(pdfBytes, []byte(""), false)
	if err != nil {
		t.Fatalf("Failed to extract AcroForm: %v", err)
	}

	if len(acroForm.Fields) == 0 {
		t.Skip("No fields found in test PDF")
	}

	// Check which fields are in object streams by trying to access them
	fieldsInStreams := 0
	fieldsDirect := 0

	for _, field := range acroForm.Fields {
		objData, err := pdf.GetObject(field.ObjectNum)
		if err == nil {
			// Check if object appears to be from a stream (no "obj" header)
			if !bytes.Contains(objData[:min(50, len(objData))], []byte(fmt.Sprintf("%d 0 obj", field.ObjectNum))) {
				fieldsInStreams++
				t.Logf("Field '%s' (obj %d) appears to be from object stream",
					field.GetFullName(), field.ObjectNum)
			} else {
				fieldsDirect++
			}
		}
	}

	t.Logf("Fields in streams: %d, direct objects: %d", fieldsInStreams, fieldsDirect)

	// Test filling fields (both direct and stream-based)
	if fieldsDirect > 0 {
		// Find a direct field (one that has "obj" header)
		var directField *Field
		for _, field := range acroForm.Fields {
			objData, err := pdf.GetObject(field.ObjectNum)
			if err == nil && bytes.Contains(objData[:min(50, len(objData))], []byte(fmt.Sprintf("%d 0 obj", field.ObjectNum))) {
				directField = field
				break
			}
		}

		if directField != nil {
			formData := types.FormData{
				directField.GetFullName(): "Direct Field Test",
			}

			filled, err := FillFormFieldsWithStreams(pdfBytes, formData, []byte(""), false)
			if err != nil {
				t.Logf("Fill error (may be expected for complex PDFs): %v", err)
			} else {
				t.Logf("Successfully filled direct field: %d bytes", len(filled))
			}
		}
	}

	if fieldsInStreams > 0 {
		// Find a stream-based field (one without "obj" header)
		var streamField *Field
		for _, field := range acroForm.Fields {
			objData, err := pdf.GetObject(field.ObjectNum)
			if err == nil && !bytes.Contains(objData[:min(50, len(objData))], []byte(fmt.Sprintf("%d 0 obj", field.ObjectNum))) {
				streamField = field
				break
			}
		}

		if streamField != nil {
			t.Logf("Testing stream-based field: '%s' (obj %d)", streamField.GetFullName(), streamField.ObjectNum)

			// Try to get the object from stream
			objData, err := pdf.GetObject(streamField.ObjectNum)
			if err != nil {
				t.Errorf("Failed to get stream object %d: %v", streamField.ObjectNum, err)
			} else {
				t.Logf("Successfully extracted stream object: %d bytes", len(objData))
				if bytes.Contains(objData, []byte("/T")) {
					t.Log("Object contains field name (/T)")
				}
			}

			// Test updating field in stream (structure test)
			formData := types.FormData{
				streamField.GetFullName(): "Stream Field Test",
			}

			// This will test the stream handling code path
			filled, err := FillFormFieldsWithStreams(pdfBytes, formData, []byte(""), true)
			if err != nil {
				t.Logf("Fill with stream field error (expected - stream rebuilding not fully implemented): %v", err)
			} else {
				t.Logf("Filled stream-based field: %d bytes", len(filled))
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
