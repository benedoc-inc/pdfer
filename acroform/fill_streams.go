// Package acroform provides form filling with object stream support
package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// FillFormFieldsWithStreams fills form fields, handling both direct objects and object streams
func FillFormFieldsWithStreams(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("PDF bytes are empty")
	}

	// Parse PDF to get encryption info and object locations
	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	var encryptInfo *types.PDFEncryption
	if pdf.Encryption() != nil {
		encryptInfo = pdf.Encryption()
	}

	// Extract AcroForm
	acroForm, err := ParseAcroForm(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	if acroForm == nil {
		return nil, fmt.Errorf("AcroForm is nil")
	}

	// Get object locations from parser
	result := make([]byte, len(pdfBytes))
	copy(result, pdfBytes)

	// Track updates per stream
	streamUpdates := make(map[int][]StreamObjectUpdate)

	for fieldName, value := range formData {
		field := acroForm.FindFieldByName(fieldName)
		if field == nil {
			if verbose {
				fmt.Printf("Warning: Field '%s' not found, skipping\n", fieldName)
			}
			continue
		}

		// Check if field is in an object stream by accessing xref
		// We need to get the object reference from the parser's internal xref
		// For now, try to get the object and check if it's accessible
		objData, objErr := pdf.GetObject(field.ObjectNum)
		if objErr != nil {
			if verbose {
				fmt.Printf("Warning: Cannot access object %d: %v, trying direct replacement\n", field.ObjectNum, objErr)
			}
			// Try direct replacement
			var fillErr error
			result, fillErr = FillFieldValue(result, field, value, encryptInfo, verbose)
			if fillErr != nil && verbose {
				fmt.Printf("Warning: Failed to fill field '%s': %v\n", fieldName, fillErr)
			}
			continue
		}

		// Check if object appears to be from a stream (no "obj" header)
		checkLen := 50
		if len(objData) < checkLen {
			checkLen = len(objData)
		}
		isInStream := !bytes.Contains(objData[:checkLen], []byte(fmt.Sprintf("%d 0 obj", field.ObjectNum)))

		if isInStream {
			// Field is in an object stream - prepare update
			updatedContent, err := updateFieldContent(objData, field, value)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: Failed to update field content: %v\n", err)
				}
				continue
			}

			// Find which stream contains this object
			streamObjNum := findStreamForObject(pdfBytes, field.ObjectNum, encryptInfo, verbose)
			if streamObjNum > 0 {
				// Get stream index from xref
				startXRef := findStartXRef(pdfBytes)
				var streamIndex int
				if startXRef >= 0 {
					xrefResult, err := parser.ParseXRefStreamFull(pdfBytes, int64(startXRef), false)
					if err == nil {
						if entry, ok := xrefResult.ObjectStreams[field.ObjectNum]; ok {
							streamIndex = entry.IndexInStream
						}
					}
				}

				// Add to stream updates
				streamUpdates[streamObjNum] = append(streamUpdates[streamObjNum], StreamObjectUpdate{
					ObjNum:     field.ObjectNum,
					Index:      streamIndex,
					NewContent: updatedContent,
				})

				if verbose {
					fmt.Printf("Prepared update for field '%s' (obj %d) in stream %d at index %d\n",
						fieldName, field.ObjectNum, streamObjNum, streamIndex)
				}
			} else {
				if verbose {
					fmt.Printf("Warning: Could not find stream for object %d, using direct replacement\n", field.ObjectNum)
				}
				// Fall back to direct replacement
				var fillErr error
				result, fillErr = FillFieldValue(result, field, value, encryptInfo, verbose)
				if fillErr != nil && verbose {
					fmt.Printf("Warning: Failed to fill field '%s': %v\n", fieldName, fillErr)
				}
			}
		} else {
			// Direct object - use simple replacement
			var fillErr error
			result, fillErr = FillFieldValue(result, field, value, encryptInfo, verbose)
			if fillErr != nil {
				if verbose {
					fmt.Printf("Warning: Failed to fill field '%s': %v\n", fieldName, fillErr)
				}
				continue
			}
		}

		if verbose {
			fmt.Printf("Filled field '%s' with value '%v'\n", fieldName, value)
		}
	}

	// Rebuild object streams that were modified
	for streamObjNum, updates := range streamUpdates {
		if verbose {
			fmt.Printf("Rebuilding object stream %d with %d updates\n", streamObjNum, len(updates))
		}

		var rebuildErr error
		result, rebuildErr = RebuildObjectStream(result, streamObjNum, updates, encryptInfo, verbose)
		if rebuildErr != nil {
			if verbose {
				fmt.Printf("Warning: Failed to rebuild object stream %d: %v\n", streamObjNum, rebuildErr)
			}
			// Continue with other streams
			continue
		}

		if verbose {
			fmt.Printf("Successfully rebuilt object stream %d\n", streamObjNum)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("result PDF is empty after filling")
	}

	return result, nil
}

// updateFieldContent updates field content with new value
func updateFieldContent(fieldData []byte, field *Field, value interface{}) ([]byte, error) {
	fieldStr := string(fieldData)
	valueStr := formatFieldValue(value, field.FT)

	// Replace or add /V entry
	vPattern := regexp.MustCompile(`/V\s*(?:\([^)]*\)|/[^\s]+|\[[^\]]*\])`)
	newV := fmt.Sprintf("/V (%s)", escapeFieldValue(valueStr))

	var newFieldStr string
	if vPattern.MatchString(fieldStr) {
		newFieldStr = vPattern.ReplaceAllString(fieldStr, newV)
	} else {
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return nil, fmt.Errorf("field dictionary not found")
		}
		newFieldStr = fieldStr[:dictEnd] + newV + " " + fieldStr[dictEnd:]
	}

	return []byte(newFieldStr), nil
}

// rebuildObjectStream rebuilds an object stream with updated objects
// This uses the RebuildObjectStream function from stream_rebuild.go
func rebuildObjectStream(pdfBytes []byte, streamObjNum int, encryptInfo *types.PDFEncryption, verbose bool) error {
	// This is called for each stream that needs rebuilding
	// The actual rebuilding is handled by RebuildObjectStream with updates
	if verbose {
		fmt.Printf("Object stream %d marked for rebuild\n", streamObjNum)
	}
	return nil
}
