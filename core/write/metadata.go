package write

import (
	"fmt"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/types"
)

// SetMetadata creates an Info dictionary object with the provided metadata
// and sets it as the document info. Returns the object number.
func (w *PDFWriter) SetMetadata(metadata *types.DocumentMetadata) int {
	if metadata == nil {
		return 0
	}

	// Build Info dictionary
	dict := Dictionary{}

	if metadata.Title != "" {
		dict["/Title"] = escapePDFStringForMetadata(metadata.Title)
	}
	if metadata.Author != "" {
		dict["/Author"] = escapePDFStringForMetadata(metadata.Author)
	}
	if metadata.Subject != "" {
		dict["/Subject"] = escapePDFStringForMetadata(metadata.Subject)
	}
	if metadata.Keywords != "" {
		dict["/Keywords"] = escapePDFStringForMetadata(metadata.Keywords)
	}
	if metadata.Creator != "" {
		dict["/Creator"] = escapePDFStringForMetadata(metadata.Creator)
	}
	if metadata.Producer != "" {
		dict["/Producer"] = escapePDFStringForMetadata(metadata.Producer)
	}
	if metadata.CreationDate != "" {
		dict["/CreationDate"] = formatPDFDate(metadata.CreationDate)
	}
	if metadata.ModDate != "" {
		dict["/ModDate"] = formatPDFDate(metadata.ModDate)
	}

	// If no dates provided, set current time as ModDate
	if metadata.ModDate == "" {
		dict["/ModDate"] = formatPDFDate(time.Now().Format(time.RFC3339))
	}

	// Add custom fields
	for key, value := range metadata.Custom {
		if key != "" && value != "" {
			// Ensure key starts with /
			if !strings.HasPrefix(key, "/") {
				key = "/" + key
			}
			dict[key] = escapePDFStringForMetadata(value)
		}
	}

	// Create the Info object
	content := w.formatDictionary(dict)
	objNum := w.AddObject(content)
	w.SetInfo(objNum)

	return objNum
}

// SetMetadataFields is a convenience method to set metadata fields individually
func (w *PDFWriter) SetMetadataFields(fields map[string]string) int {
	metadata := &types.DocumentMetadata{
		Custom: make(map[string]string),
	}

	for key, value := range fields {
		switch strings.ToLower(key) {
		case "title":
			metadata.Title = value
		case "author":
			metadata.Author = value
		case "subject":
			metadata.Subject = value
		case "keywords":
			metadata.Keywords = value
		case "creator":
			metadata.Creator = value
		case "producer":
			metadata.Producer = value
		case "creationdate", "creation_date":
			metadata.CreationDate = value
		case "moddate", "mod_date":
			metadata.ModDate = value
		default:
			// Custom field
			metadata.Custom[key] = value
		}
	}

	return w.SetMetadata(metadata)
}

// escapePDFStringForMetadata escapes a string for use in PDF metadata (returns content without parentheses)
func escapePDFStringForMetadata(s string) string {
	// PDF strings use parentheses and need escaping
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// formatPDFDate formats a date string for PDF
// Accepts ISO 8601 format (YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS) or PDF format (D:YYYYMMDDHHmmSSOHH'mm)
// Returns PDF date format: D:YYYYMMDDHHmmSSOHH'mm
func formatPDFDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	// If already in PDF format, return as-is
	if strings.HasPrefix(dateStr, "D:") {
		return escapePDFString(dateStr)
	}

	// Try to parse as ISO 8601 or other common formats
	var t time.Time
	var err error

	// Try ISO 8601 with time
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		t, err = time.Parse(format, dateStr)
		if err == nil {
			break
		}
	}

	// If parsing failed, use current time
	if err != nil {
		t = time.Now()
	}

	// Format as PDF date: D:YYYYMMDDHHmmSSOHH'mm
	// O is timezone offset: +HH or -HH
	// 'mm' is minutes offset (usually 00)
	year := t.Year()
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	min := t.Minute()
	sec := t.Second()

	// Get timezone offset
	_, offset := t.Zone()
	offsetHours := offset / 3600
	offsetMins := (offset % 3600) / 60

	// Format timezone
	var tzStr string
	if offsetHours >= 0 {
		tzStr = fmt.Sprintf("+%02d'%02d", offsetHours, offsetMins)
	} else {
		tzStr = fmt.Sprintf("-%02d'%02d", -offsetHours, -offsetMins)
	}

	pdfDate := fmt.Sprintf("D:%04d%02d%02d%02d%02d%02d%s",
		year, month, day, hour, min, sec, tzStr)

	// Return as escaped string (formatValue will add parentheses)
	return escapePDFStringForMetadata(pdfDate)
}

// NewMetadataFromFields creates a DocumentMetadata from a map of fields
func NewMetadataFromFields(fields map[string]string) *types.DocumentMetadata {
	metadata := &types.DocumentMetadata{
		Custom: make(map[string]string),
	}

	for key, value := range fields {
		switch strings.ToLower(key) {
		case "title":
			metadata.Title = value
		case "author":
			metadata.Author = value
		case "subject":
			metadata.Subject = value
		case "keywords":
			metadata.Keywords = value
		case "creator":
			metadata.Creator = value
		case "producer":
			metadata.Producer = value
		case "creationdate", "creation_date":
			metadata.CreationDate = value
		case "moddate", "mod_date":
			metadata.ModDate = value
		default:
			metadata.Custom[key] = value
		}
	}

	return metadata
}
