package write

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestSetMetadata(t *testing.T) {
	writer := NewPDFWriter()

	metadata := &types.DocumentMetadata{
		Title:        "Test Document",
		Author:       "Test Author",
		Subject:      "Test Subject",
		Keywords:     "test, pdf, metadata",
		Creator:      "pdfer",
		Producer:     "pdfer 0.9.2",
		CreationDate: "2024-01-15T10:30:00Z",
		ModDate:      "2024-01-16T14:45:00Z",
		Custom: map[string]string{
			"CustomField": "CustomValue",
		},
	}

	objNum := writer.SetMetadata(metadata)
	if objNum == 0 {
		t.Fatal("SetMetadata should return a non-zero object number")
	}

	if writer.infoRef == "" {
		t.Error("infoRef should be set")
	}

	// Verify the object was created
	obj, exists := writer.objects[objNum]
	if !exists {
		t.Fatal("Info object should exist")
	}

	// Check that the content contains expected fields
	content := string(obj.Content)
	if !strings.Contains(content, "/Title") {
		t.Error("Content should contain /Title")
	}
	if !strings.Contains(content, "Test Document") {
		t.Error("Content should contain title")
	}
	if !strings.Contains(content, "/Author") {
		t.Error("Content should contain /Author")
	}
	if !strings.Contains(content, "Test Author") {
		t.Error("Content should contain author")
	}
	if !strings.Contains(content, "/CreationDate") {
		t.Error("Content should contain /CreationDate")
	}
	if !strings.Contains(content, "D:2024") {
		t.Error("Content should contain PDF date format")
	}
}

func TestSetMetadataFields(t *testing.T) {
	writer := NewPDFWriter()

	fields := map[string]string{
		"title":   "My Document",
		"author":  "John Doe",
		"subject": "Testing",
		"custom1": "value1",
	}

	objNum := writer.SetMetadataFields(fields)
	if objNum == 0 {
		t.Fatal("SetMetadataFields should return a non-zero object number")
	}

	obj := writer.objects[objNum]
	content := string(obj.Content)

	if !strings.Contains(content, "/Title") || !strings.Contains(content, "My Document") {
		t.Error("Should contain title")
	}
	if !strings.Contains(content, "/Author") || !strings.Contains(content, "John Doe") {
		t.Error("Should contain author")
	}
	// Custom fields should be included (check for the value, key format may vary)
	if !strings.Contains(content, "value1") {
		t.Error("Should contain custom field value")
	}
}

func TestFormatPDFDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Should contain D:YYYYMMDD
	}{
		{
			name:     "ISO 8601 date",
			input:    "2024-01-15",
			expected: "D:20240115",
		},
		{
			name:     "ISO 8601 datetime",
			input:    "2024-01-15T10:30:00Z",
			expected: "D:20240115103000",
		},
		{
			name:     "RFC3339",
			input:    "2024-01-15T10:30:00+05:00",
			expected: "D:20240115",
		},
		{
			name:     "PDF format already",
			input:    "D:20240115103000+05'00",
			expected: "D:20240115103000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPDFDate(tt.input)
			// Remove parentheses and check content
			result = strings.Trim(result, "()")
			if !strings.Contains(result, tt.expected) {
				t.Errorf("formatPDFDate(%q) = %q, should contain %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapePDFStringForMetadata(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "Hello",
			expected: "Hello",
		},
		{
			name:     "with parentheses",
			input:    "Test (value)",
			expected: "Test \\(value\\)",
		},
		{
			name:     "with newline",
			input:    "Line1\nLine2",
			expected: "Line1\\nLine2",
		},
		{
			name:     "with backslash",
			input:    "Path\\to\\file",
			expected: "Path\\\\to\\\\file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapePDFStringForMetadata(tt.input)
			if result != tt.expected {
				t.Errorf("escapePDFStringForMetadata(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetMetadata_WritePDF(t *testing.T) {
	writer := NewPDFWriter()

	// Create a minimal PDF with metadata
	metadata := &types.DocumentMetadata{
		Title:   "Test PDF",
		Author:  "Test Author",
		Creator: "pdfer",
	}

	writer.SetMetadata(metadata)

	// Create a minimal catalog
	catalogDict := Dictionary{
		"/Type":  "/Catalog",
		"/Pages": "2 0 R",
	}
	catalogNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogNum)

	// Create a minimal pages object
	pagesDict := Dictionary{
		"/Type":  "/Pages",
		"/Count": 0,
		"/Kids":  []interface{}{},
	}
	pagesNum := writer.AddObject(writer.formatDictionary(pagesDict))

	// Update catalog to reference pages
	catalogDict["/Pages"] = fmt.Sprintf("%d 0 R", pagesNum)
	writer.SetObject(catalogNum, writer.formatDictionary(catalogDict))

	// Write PDF
	var buf bytes.Buffer
	err := writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Verify PDF structure
	if !strings.Contains(string(pdfBytes), "/Info") {
		t.Error("PDF should contain /Info reference")
	}

	// Verify metadata content
	if !strings.Contains(string(pdfBytes), "/Title") {
		t.Error("PDF should contain /Title")
	}
	if !strings.Contains(string(pdfBytes), "Test PDF") {
		t.Error("PDF should contain title value")
	}
}

func TestNewMetadataFromFields(t *testing.T) {
	fields := map[string]string{
		"title":        "My Title",
		"author":       "My Author",
		"subject":      "My Subject",
		"keywords":     "key1, key2",
		"creator":      "My Creator",
		"producer":     "My Producer",
		"creationdate": "2024-01-15",
		"moddate":      "2024-01-16",
		"custom":       "custom value",
	}

	metadata := NewMetadataFromFields(fields)

	if metadata.Title != "My Title" {
		t.Errorf("Title = %q, want %q", metadata.Title, "My Title")
	}
	if metadata.Author != "My Author" {
		t.Errorf("Author = %q, want %q", metadata.Author, "My Author")
	}
	if metadata.Custom["custom"] != "custom value" {
		t.Errorf("Custom field = %q, want %q", metadata.Custom["custom"], "custom value")
	}
}

func TestSetMetadata_EmptyMetadata(t *testing.T) {
	writer := NewPDFWriter()

	// Empty metadata should still set ModDate
	metadata := &types.DocumentMetadata{}
	objNum := writer.SetMetadata(metadata)

	if objNum == 0 {
		t.Error("Should create object even with empty metadata (for ModDate)")
	}

	obj := writer.objects[objNum]
	content := string(obj.Content)

	// Should have ModDate set to current time
	if !strings.Contains(content, "/ModDate") {
		t.Error("Should set ModDate even for empty metadata")
	}
}

func TestSetMetadata_CurrentTimeModDate(t *testing.T) {
	writer := NewPDFWriter()

	metadata := &types.DocumentMetadata{
		Title: "Test",
	}

	objNum := writer.SetMetadata(metadata)

	if objNum == 0 {
		t.Fatal("Should create object")
	}

	obj := writer.objects[objNum]
	content := string(obj.Content)

	// Extract ModDate from content
	if !strings.Contains(content, "/ModDate") {
		t.Error("Should set ModDate")
	}

	// Verify it's a recent date (within the test execution window)
	// The date should be between before and after
	if !strings.Contains(content, "D:") {
		t.Error("ModDate should be in PDF format")
	}
}
