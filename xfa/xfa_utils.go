package xfa

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// findObjectBoundaries finds the start and end of a PDF object using incremental parsing
// This uses non-encrypted markers ("obj", "endobj") to locate object boundaries
func findObjectBoundaries(pdfBytes []byte, objIndex int, objNum int, objName string, verbose bool) (objStart, objEnd int, objContent []byte, err error) {
	// Find object boundaries - search backwards from objIndex to find "obj" keyword
	// The xref offset might point to encrypted content, but "obj" keyword is not encrypted
	// Strategy: First try direct pattern search, then fall back to searching for "obj" keyword

	objKeywordPos := -1
	objStart = objIndex

	// First, try to find the object number pattern directly (works if object header is visible)
	// This is the most reliable method
	objPattern := []byte(fmt.Sprintf("%d 0 obj", objNum))

	// Search in a window around the given index first
	if objIndex >= 0 && objIndex < len(pdfBytes) {
		searchStart := max(0, objIndex-2000)
		searchEnd := min(len(pdfBytes), objIndex+2000)
		searchArea := pdfBytes[searchStart:searchEnd]

		patternPos := bytes.Index(searchArea, objPattern)
		if patternPos != -1 {
			objKeywordPos = searchStart + patternPos + len(fmt.Sprintf("%d 0 ", objNum))
			objStart = searchStart + patternPos
			// Find line start
			for objStart > 0 && pdfBytes[objStart-1] != '\n' && pdfBytes[objStart-1] != '\r' {
				objStart--
			}
		}
	}

	// If pattern not found in window, the object might be compressed or the offset wrong
	// Try searching entire PDF (slower but more reliable)
	if objKeywordPos == -1 {
		patternPos := bytes.Index(pdfBytes, objPattern)
		if patternPos != -1 {
			objKeywordPos = patternPos + len(fmt.Sprintf("%d 0 ", objNum))
			objStart = patternPos
			// Find line start
			for objStart > 0 && pdfBytes[objStart-1] != '\n' && pdfBytes[objStart-1] != '\r' {
				objStart--
			}
		} else {
			// Object header not found - might be compressed or offset is wrong
			// Try searching for just the number "212" followed by whitespace and "0" near the offset
			// This handles cases where the object header format is slightly different
			numStr := fmt.Sprintf("%d", objNum)
			searchStart := max(0, objIndex-1000)
			searchEnd := min(len(pdfBytes), objIndex+1000)
			searchArea := pdfBytes[searchStart:searchEnd]

			// Look for number pattern
			numBytes := []byte(numStr)
			numPos := bytes.Index(searchArea, numBytes)
			if numPos != -1 {
				// Found the number, check if followed by space/tab and "0 obj"
				checkPos := searchStart + numPos + len(numBytes)
				if checkPos+10 < len(pdfBytes) {
					afterNum := pdfBytes[checkPos : checkPos+10]
					// Check for whitespace followed by "0 obj"
					if afterNum[0] == ' ' || afterNum[0] == '\t' || afterNum[0] == '\n' || afterNum[0] == '\r' {
						if bytes.HasPrefix(afterNum[1:], []byte("0 obj")) {
							objKeywordPos = checkPos + 1 + 2 // Position after "0 "
							objStart = searchStart + numPos
							// Find line start
							for objStart > 0 && pdfBytes[objStart-1] != '\n' && pdfBytes[objStart-1] != '\r' {
								objStart--
							}
						}
					}
				}
			}
		}
	}

	// If still not found, try searching for "obj" keyword near the offset and verify object number
	if objKeywordPos == -1 {
		searchWindow := 5000 // Very large window for encrypted PDFs
		searchStart := max(0, objIndex-searchWindow)
		searchEnd := min(len(pdfBytes), objIndex+searchWindow)
		searchArea := pdfBytes[searchStart:searchEnd]

		// Search for "obj" keyword (not encrypted)
		objKeywordPosInArea := bytes.Index(searchArea, []byte("obj"))
		if objKeywordPosInArea != -1 {
			objKeywordPos = searchStart + objKeywordPosInArea
			// Search backwards to find the start of the object number line
			for i := objKeywordPos - 1; i >= 0 && i > objKeywordPos-100; i-- {
				if pdfBytes[i] >= '0' && pdfBytes[i] <= '9' {
					lineStart := i
					for lineStart > 0 && pdfBytes[lineStart-1] != '\n' && pdfBytes[lineStart-1] != '\r' {
						lineStart--
					}
					// Verify this is the right object by checking the number
					lineEnd := objKeywordPos + 3
					for lineEnd < len(pdfBytes) && pdfBytes[lineEnd] != '\n' && pdfBytes[lineEnd] != '\r' {
						lineEnd++
					}
					lineContent := string(pdfBytes[lineStart:lineEnd])
					if bytes.Contains([]byte(lineContent), []byte(fmt.Sprintf("%d 0 obj", objNum))) {
						objStart = lineStart
						break
					}
				}
			}
		}
	}

	// If still not found, the object header might be missing or the object is compressed
	// UniPDF approach: use the xref offset directly and decrypt content there
	// The xref offset points to the start of encrypted content, so start there
	// But we should also try searching backwards a bit in case there's a header
	if objKeywordPos == -1 {
		// For encrypted PDFs without visible headers, UniPDF decrypts at the xref offset
		// Try starting slightly before the offset (in case there's a short header)
		// Then decrypt a larger chunk to ensure we get the full object
		searchBack := 200 // Search backwards for potential header
		objStart = max(0, objIndex-searchBack)
		estimatedSize := 10000 // Larger estimate for AcroForm with XFA
		objEnd = min(len(pdfBytes), objIndex+estimatedSize)
		objContent = pdfBytes[objStart:objEnd]

		if verbose {
			log.Printf("%s object %d: Using xref offset with search window (start=%d, xref=%d, end=%d) - header not found, will decrypt and look for structure",
				objName, objNum, objStart, objIndex, objEnd)
		}
	} else {
		// Found obj keyword - find endobj normally
		// Find "endobj" - search forward from objStart
		objEndBytes := bytes.Index(pdfBytes[objStart:], []byte("endobj"))
		if objEndBytes == -1 {
			return 0, 0, nil, fmt.Errorf("%s object %d end not found", objName, objNum)
		}
		objEnd = objStart + objEndBytes + 6 // Include "endobj"
		objContent = pdfBytes[objStart:objEnd]
	}

	return objStart, objEnd, objContent, nil
}

// extractDictionaryKeys extracts PDF dictionary keys for debugging
func extractDictionaryKeys(dictContent []byte) string {
	var keys []string
	// Simple extraction: look for /Key patterns
	content := string(dictContent)
	keyPattern := regexp.MustCompile(`/([A-Za-z][A-Za-z0-9]*)`)
	matches := keyPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			keys = append(keys, match[1])
		}
	}
	if len(keys) > 0 {
		return strings.Join(keys, ", ")
	}
	return "none found"
}

// findAndDecryptAcroForm finds and decrypts the AcroForm object, returning the decrypted content
func findAndDecryptAcroForm(pdfBytes []byte, acroFormObjNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Find the AcroForm object
	objIndex, err := parser.FindObjectByNumber(pdfBytes, acroFormObjNum, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("AcroForm object %d not found: %v", acroFormObjNum, err)
	}

	// Find object boundaries using incremental parsing
	objStart, objEnd, objContent, err := findObjectBoundaries(pdfBytes, objIndex, acroFormObjNum, "AcroForm", verbose)
	if err != nil {
		return nil, err
	}

	if verbose {
		log.Printf("AcroForm object %d: objStart=%d, objEnd=%d, content length=%d", acroFormObjNum, objStart, objEnd, len(objContent))
	}

	// Decrypt if needed - only decrypt the encrypted portions between markers
	var decryptedContent []byte
	if encryptInfo != nil {
		// Find object structure markers (these are NOT encrypted)
		objKeywordPos := bytes.Index(objContent, []byte("obj"))

		// Extract generation number from object header (if available)
		genNum := 0
		if objKeywordPos != -1 {
			genNum = extractGenerationNumber(objContent, acroFormObjNum)
			if verbose {
				log.Printf("Extracted generation number for AcroForm object %d: %d", acroFormObjNum, genNum)
			}
		} else {
			// Object header not found - try to extract from objContent by searching for pattern
			genNum = extractGenerationNumber(objContent, acroFormObjNum)
			if genNum == 0 {
				if verbose {
					log.Printf("AcroForm object header not found, using default generation number 0")
				}
				// Note: If decryption fails, we might need to try other generation numbers
				// but that would be expensive. Most PDFs use generation 0.
			} else if verbose {
				log.Printf("Found generation number %d for AcroForm object %d (header not in expected location)", genNum, acroFormObjNum)
			}
		}

		var dataStart int
		if objKeywordPos == -1 {
			// Object header not found - UniPDF approach: try decrypting from multiple starting points
			// The xref offset might point to encrypted content, but alignment is unknown
			// Try starting from the beginning of objContent and let decryptInChunks find alignment
			// Also try starting from the xref offset position
			relativeXrefOffset := objIndex - objStart
			if relativeXrefOffset < 0 {
				relativeXrefOffset = 0
			}

			// Try decrypting from start first (decryptInChunks will try alignments)
			dataStart = 0
			if verbose {
				log.Printf("AcroForm object header not found, trying decryption from start of content (xref at relative offset %d)", relativeXrefOffset)
			}
		} else {
			// Skip "obj" and whitespace to find where encrypted data starts
			dataStart = objKeywordPos + 3
			for dataStart < len(objContent) && (objContent[dataStart] == ' ' || objContent[dataStart] == '\r' || objContent[dataStart] == '\n' || objContent[dataStart] == '\t') {
				dataStart++
			}
		}

		// Check if this is a stream object
		streamKeywordPos := bytes.Index(objContent, []byte("stream"))
		isStreamObject := streamKeywordPos != -1

		if isStreamObject {
			// Stream object: decrypt dictionary portion (between "obj" and "stream")
			// The dictionary is encrypted, but "stream", stream data, "endstream", and "endobj" are not
			if verbose {
				log.Printf("AcroForm object is a stream object, decrypting dictionary portion only")
			}

			// Dictionary is encrypted from dataStart to streamKeywordPos
			encryptedDict := objContent[dataStart:streamKeywordPos]

			// Decrypt dictionary in chunks if needed (for alignment)
			// Use extracted generation number
			decryptedDict, err := decryptInChunks(encryptedDict, acroFormObjNum, genNum, encryptInfo, verbose)
			if err != nil {
				if verbose {
					log.Printf("Failed to decrypt dictionary: %v", err)
				}
				decryptedContent = objContent
			} else {
				// Reconstruct: object header + decrypted dictionary + "stream" + stream data + "endstream" + "endobj"
				objectHeader := objContent[:dataStart]
				streamAndRest := objContent[streamKeywordPos:]
				decryptedContent = append(objectHeader, decryptedDict...)
				decryptedContent = append(decryptedContent, streamAndRest...)
				if verbose {
					log.Printf("Decrypted dictionary: %d bytes -> %d bytes", len(encryptedDict), len(decryptedDict))
				}
			}
		} else {
			// Regular object: decrypt content between dataStart and "endobj"
			endobjPos := bytes.Index(objContent, []byte("endobj"))
			if endobjPos == -1 {
				// If endobj not found and no header, try decrypting a reasonable chunk
				// UniPDF approach: decrypt and look for structure markers
				if objKeywordPos == -1 {
					// No header, no endobj - decrypt a chunk starting from xref offset
					// UniPDF approach: the xref offset points to encrypted content
					// Calculate where the xref offset is in objContent
					relativeXrefOffset := objIndex - objStart
					if relativeXrefOffset < 0 {
						relativeXrefOffset = 0
					}

					// Try decrypting from the xref offset position
					// decryptInChunks will try different alignments, but we should start from xref position
					decryptStart := relativeXrefOffset
					decryptLen := min(10000, len(objContent)-decryptStart)
					// Round down to multiple of 16 for AES
					decryptLen = (decryptLen / 16) * 16
					if decryptLen >= 16 {
						encryptedContent := objContent[decryptStart : decryptStart+decryptLen]
						if verbose {
							log.Printf("Attempting to decrypt %d bytes starting from xref position (offset %d in content, absolute %d)",
								decryptLen, decryptStart, objIndex)
						}
						decryptedData, err := decryptInChunks(encryptedContent, acroFormObjNum, genNum, encryptInfo, verbose)
						if err == nil && len(decryptedData) > 0 {
							// If decryptInChunks succeeded, use the decrypted content
							// It will have found valid PDF structure (<< or PDF keywords)
							decryptedContent = decryptedData
							if verbose {
								log.Printf("Successfully decrypted AcroForm (no header/endobj): %d bytes", len(decryptedContent))

								// Output full structure for debugging
								log.Printf("=== DECRYPTED ACROFORM STRUCTURE ===")
								log.Printf("Content length: %d bytes", len(decryptedContent))

								// Hex dump of first 2000 bytes
								previewLen := min(2000, len(decryptedContent))
								log.Printf("First %d bytes (hex dump):", previewLen)
								for i := 0; i < previewLen; i += 16 {
									end := min(i+16, previewLen)
									hexPart := fmt.Sprintf("%04x: ", i)
									for j := i; j < end; j++ {
										hexPart += fmt.Sprintf("%02x ", decryptedContent[j])
									}
									log.Printf("%s", hexPart)
								}

								// ASCII dump
								log.Printf("First %d bytes (ASCII, non-printable as .):", previewLen)
								asciiLine := ""
								for i := 0; i < previewLen; i++ {
									if i > 0 && i%80 == 0 {
										log.Printf("%s", asciiLine)
										asciiLine = ""
									}
									b := decryptedContent[i]
									if b >= 32 && b < 127 {
										asciiLine += string(b)
									} else {
										asciiLine += "."
									}
								}
								if asciiLine != "" {
									log.Printf("%s", asciiLine)
								}

								// Check for PDF markers
								if bytes.Contains(decryptedContent, []byte("<<")) {
									dictStart := bytes.Index(decryptedContent, []byte("<<"))
									log.Printf("Found '<<' at offset %d", dictStart)
									// Show context around dictionary start
									ctxStart := max(0, dictStart-50)
									ctxEnd := min(len(decryptedContent), dictStart+200)
									log.Printf("Context around '<<' (offset %d-%d): %q", ctxStart, ctxEnd, string(decryptedContent[ctxStart:ctxEnd]))
								}
								if bytes.Contains(decryptedContent, []byte("/XFA")) {
									xfaPos := bytes.Index(decryptedContent, []byte("/XFA"))
									log.Printf("Found '/XFA' at offset %d", xfaPos)
								} else {
									log.Printf("'/XFA' NOT found in decrypted content")
									// Check for XFA with different case/spacing
									if bytes.Contains(decryptedContent, []byte("XFA")) || bytes.Contains(decryptedContent, []byte("xfa")) {
										log.Printf("Found 'XFA' (case-insensitive) in decrypted content")
									}
								}
								log.Printf("=== END DECRYPTED ACROFORM STRUCTURE ===")
							}
						} else {
							if verbose {
								log.Printf("Failed to decrypt: %v", err)
							}
							decryptedContent = objContent
						}
					} else {
						decryptedContent = objContent
					}
				} else {
					// Has header but no endobj - decrypt up to end
					endobjPos = len(objContent)
					encryptedContent := objContent[dataStart:endobjPos]
					decryptedData, err := decryptInChunks(encryptedContent, acroFormObjNum, genNum, encryptInfo, verbose)
					if err != nil {
						if verbose {
							log.Printf("Failed to decrypt object content: %v", err)
						}
						decryptedContent = objContent
					} else {
						objectHeader := objContent[:dataStart]
						decryptedContent = append(objectHeader, decryptedData...)
					}
				}
			} else {
				// Has endobj - decrypt normally
				encryptedContent := objContent[dataStart:endobjPos]
				decryptedData, err := decryptInChunks(encryptedContent, acroFormObjNum, genNum, encryptInfo, verbose)
				if err != nil {
					if verbose {
						log.Printf("Failed to decrypt object content: %v", err)
					}
					decryptedContent = objContent
				} else {
					// Reconstruct: object header (if exists) + decrypted content + "endobj" (if exists)
					if objKeywordPos != -1 {
						objectHeader := objContent[:dataStart]
						decryptedContent = append(objectHeader, decryptedData...)
						decryptedContent = append(decryptedContent, []byte("endobj")...)
					} else {
						// No header - just use decrypted content
						decryptedContent = decryptedData
						if verbose {
							log.Printf("Decrypted content (no header): %d bytes, looking for dictionary markers", len(decryptedContent))
							if bytes.Contains(decryptedContent, []byte("<<")) {
								log.Printf("Found dictionary markers in decrypted content")
							}
						}
					}
				}
			}
		}
	} else {
		decryptedContent = objContent
	}

	return decryptedContent, nil
}

// decryptInChunks decrypts data in chunks, trying different alignments to find the correct decryption
// This handles cases where encrypted data might not start exactly at a 16-byte boundary
func decryptInChunks(encryptedData []byte, objNum, genNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	if len(encryptedData) == 0 {
		return encryptedData, nil
	}

	// For AES, we need to find where the actual encrypted data starts (16-byte aligned)
	// UniPDF approach: try many alignment offsets and verify decryption produces valid PDF
	if encryptInfo != nil && (encryptInfo.V == 4 || encryptInfo.V == 5) {
		// Try different start offsets to find correct alignment
		// Encrypted data should be a multiple of 16 bytes (including IV)
		// Try up to 200 bytes of alignment (UniPDF tries many offsets)
		maxOffset := min(200, len(encryptedData))
		for offset := 0; offset < maxOffset; offset++ {
			testStart := offset
			testEnd := len(encryptedData)
			testLen := testEnd - testStart

			// Must be at least 16 bytes (for IV) and multiple of 16 for AES
			if testLen < 16 || testLen%16 != 0 {
				continue
			}

			testData := encryptedData[testStart:testEnd]
			decrypted, err := encryption.DecryptObject(testData, objNum, genNum, encryptInfo)
			if err == nil && len(decrypted) > 0 {
				// Verify decryption produced readable PDF content
				// UniPDF approach: look for valid PDF dictionary structure, not just markers
				hasDictMarker := bytes.Contains(decrypted, []byte("<<"))
				hasDictEnd := bytes.Contains(decrypted, []byte(">>"))
				hasValidDict := hasDictMarker && hasDictEnd

				// Check for PDF dictionary keys (more reliable than just <<)
				hasPDFKeywords := bytes.Contains(decrypted, []byte("/")) &&
					(bytes.Contains(decrypted, []byte("/XFA")) ||
						bytes.Contains(decrypted, []byte("/Fields")) ||
						bytes.Contains(decrypted, []byte("/AcroForm")))

				// Also check for common PDF dictionary keys
				hasPDFDict := bytes.Contains(decrypted, []byte("/Type")) ||
					bytes.Contains(decrypted, []byte("/Subtype")) ||
					bytes.Contains(decrypted, []byte("/Kids"))

				// Require both dictionary markers AND at least one PDF keyword for validity
				// This prevents false positives from binary data that happens to contain "<<"
				isValidPDF := hasValidDict && (hasPDFKeywords || hasPDFDict)

				// Also check if decrypted content has readable ASCII PDF structure
				// Count printable ASCII bytes - valid PDF should have many
				printableCount := 0
				for _, b := range decrypted {
					if b >= 32 && b < 127 {
						printableCount++
					}
				}
				printableRatio := float64(printableCount) / float64(len(decrypted))
				hasReadableContent := printableRatio > 0.3 // At least 30% printable (PDF dictionaries are mostly ASCII)

				if isValidPDF || (hasPDFKeywords && hasReadableContent) {
					if verbose {
						log.Printf("Found valid decryption at offset %d, decrypted length=%d (hasDict=%v, hasKeywords=%v, printable=%.1f%%)",
							offset, len(decrypted), hasValidDict, hasPDFKeywords, printableRatio*100)
						if len(decrypted) > 0 && len(decrypted) <= 200 {
							log.Printf("Decrypted preview: %q", decrypted)
						} else if len(decrypted) > 200 {
							// Show a sample from the middle where dictionary might be
							midStart := max(0, len(decrypted)/2-100)
							midEnd := min(len(decrypted), midStart+200)
							log.Printf("Decrypted preview (middle section): %q", decrypted[midStart:midEnd])
						}
					}
					// Prepend any skipped bytes (non-encrypted header)
					if offset > 0 {
						result := make([]byte, offset+len(decrypted))
						copy(result, encryptedData[:offset])
						copy(result[offset:], decrypted)
						return result, nil
					}
					return decrypted, nil
				} else if verbose && offset == 0 {
					// Log first attempt to see what we're getting
					log.Printf("Decryption at offset %d succeeded but no PDF markers found, length=%d, preview: %q",
						offset, len(decrypted), decrypted[:min(100, len(decrypted))])
				}
			}
		}

		// If no valid decryption found, try direct decryption anyway
		decrypted, err := encryption.DecryptObject(encryptedData, objNum, genNum, encryptInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt: %v", err)
		}
		return decrypted, nil
	}

	// For RC4 or no encryption, decrypt directly
	decrypted, err := encryption.DecryptObject(encryptedData, objNum, genNum, encryptInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %v", err)
	}
	return decrypted, nil
}

// findXFAArrayContent extracts the XFA array content string from decrypted AcroForm content
func findXFAArrayContent(decryptedContent []byte, verbose bool) (string, error) {
	// UniPDF approach: the decrypted content might have padding or extra data
	// Find where the actual PDF dictionary starts (look for "<<")
	dictStart := bytes.Index(decryptedContent, []byte("<<"))
	var xfaPos int = -1

	if dictStart != -1 {
		// Found dictionary start - find matching >> with proper nesting
		// Must balance << and >> pairs for nested dictionaries
		dictEnd := -1
		depth := 0
		for i := dictStart; i < len(decryptedContent)-1; i++ {
			if decryptedContent[i] == '<' && decryptedContent[i+1] == '<' {
				depth++
				i++ // Skip second '<'
			} else if decryptedContent[i] == '>' && decryptedContent[i+1] == '>' {
				depth--
				if depth == 0 {
					dictEnd = i
					break
				}
				i++ // Skip second '>'
			}
		}
		if dictEnd != -1 {
			dictContent := decryptedContent[dictStart : dictEnd+2]
			if verbose {
				log.Printf("Found dictionary start at offset %d, end at %d (length: %d bytes)", dictStart, dictEnd, len(dictContent))
				// Show the actual dictionary content to verify it's valid
				previewLen := min(200, len(dictContent))
				log.Printf("Dictionary content preview: %q", dictContent[:previewLen])
			}

			// Search for /XFA within the dictionary
			xfaPos = bytes.Index(dictContent, []byte("/XFA"))
			if xfaPos != -1 {
				xfaPos += dictStart // Adjust to absolute position
			} else {
				// Try case variations
				xfaPos = bytes.Index(dictContent, []byte("/xfa"))
				if xfaPos != -1 {
					xfaPos += dictStart
				}
			}

			if xfaPos != -1 {
				if verbose {
					log.Printf("Found /XFA at offset %d within dictionary", xfaPos)
				}
			} else if verbose {
				// Dictionary found but no /XFA - show what's in the dictionary
				log.Printf("Dictionary found but /XFA not found. Dictionary keys: %q", extractDictionaryKeys(dictContent))
			}
		} else if verbose {
			log.Printf("Found '<<' at offset %d but no matching '>>' found - might be false positive", dictStart)
		}
	}

	// If not found from dictionary, try searching entire content
	if xfaPos == -1 {
		xfaPos = bytes.Index(decryptedContent, []byte("/XFA"))
	}
	if xfaPos == -1 {
		// Try case variations
		xfaPos = bytes.Index(decryptedContent, []byte("/xfa"))
	}
	if xfaPos == -1 {
		// Try with spaces
		xfaPos = bytes.Index(decryptedContent, []byte("/ XFA"))
	}
	if xfaPos == -1 {
		// Try searching byte by byte
		for i := 0; i < len(decryptedContent)-3; i++ {
			if decryptedContent[i] == '/' &&
				(decryptedContent[i+1] == 'X' || decryptedContent[i+1] == 'x') &&
				(decryptedContent[i+2] == 'F' || decryptedContent[i+2] == 'f') &&
				(decryptedContent[i+3] == 'A' || decryptedContent[i+3] == 'a') {
				xfaPos = i
				break
			}
		}
	}

	if xfaPos == -1 {
		if verbose {
			log.Printf("XFA not found in decrypted content (length: %d bytes)", len(decryptedContent))
			// Show content around dictionary start if found
			if dictStart != -1 {
				previewStart := max(0, dictStart-50)
				previewEnd := min(len(decryptedContent), dictStart+500)
				log.Printf("Content around dictionary start (offset %d): %q", dictStart, decryptedContent[previewStart:previewEnd])
			} else {
				log.Printf("Decrypted content preview (first 500 bytes): %q", decryptedContent[:min(500, len(decryptedContent))])
			}
		}
		return "", fmt.Errorf("XFA entry not found in AcroForm")
	}

	// Find array after /XFA
	arrayStart := bytes.Index(decryptedContent[xfaPos:], []byte("["))
	if arrayStart == -1 {
		return "", fmt.Errorf("XFA array start not found after /XFA")
	}
	arrayStartIdx := xfaPos + arrayStart + 1 // Position after '['

	// Find matching ']' for the array
	// Start at depth 1 because we've already passed the opening '['
	depth := 1
	arrayEndIdx := arrayStartIdx
	for i := arrayStartIdx; i < len(decryptedContent) && i < arrayStartIdx+10000; i++ {
		if decryptedContent[i] == '[' {
			depth++
		} else if decryptedContent[i] == ']' {
			depth--
			if depth == 0 {
				arrayEndIdx = i + 1
				break
			}
		}
	}

	if arrayEndIdx == arrayStartIdx {
		return "", fmt.Errorf("could not find end of XFA array")
	}

	return string(decryptedContent[arrayStartIdx:arrayEndIdx]), nil
}

// extractGenerationNumber extracts the generation number from an object header
// Object format: "objNum genNum obj"
// Returns the generation number, or 0 if not found (most objects are generation 0)
func extractGenerationNumber(objContent []byte, objNum int) int {
	// Find object header pattern: "objNum genNum obj"
	// Try multiple patterns to handle different whitespace
	patterns := []*regexp.Regexp{
		regexp.MustCompile(fmt.Sprintf(`%d\s+(\d+)\s+obj`, objNum)), // Standard: "123 0 obj"
		regexp.MustCompile(fmt.Sprintf(`%d\s+(\d+)\s+obj`, objNum)), // With tabs
		regexp.MustCompile(fmt.Sprintf(`%d\s*(\d+)\s*obj`, objNum)), // Flexible whitespace
	}

	for _, pattern := range patterns {
		match := pattern.FindSubmatch(objContent)
		if match != nil && len(match) > 1 {
			genNum, err := strconv.Atoi(string(match[1]))
			if err == nil {
				return genNum
			}
		}
	}

	// Also try searching in a larger window if objContent is large
	// Sometimes the header might be further in if there's padding
	if len(objContent) > 1000 {
		// Search first 500 bytes more carefully
		searchArea := objContent[:min(500, len(objContent))]
		for _, pattern := range patterns {
			match := pattern.FindSubmatch(searchArea)
			if match != nil && len(match) > 1 {
				genNum, err := strconv.Atoi(string(match[1]))
				if err == nil {
					return genNum
				}
			}
		}
	}

	// Default to 0 if not found (most PDF objects are generation 0)
	return 0
}

// This follows the same approach as findAndDecryptAcroForm: parse structure first, then decrypt only encrypted portions
func extractStreamFromPDF(pdfBytes []byte, streamObjNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, int, error) {
	// Find the stream object using incremental parser (finds non-encrypted markers)
	streamObjIndex, err := parser.FindObjectByNumber(pdfBytes, streamObjNum, encryptInfo, verbose)
	if err != nil {
		return nil, 0, fmt.Errorf("stream object %d not found: %v", streamObjNum, err)
	}

	// Find object boundaries using incremental parsing
	objStart, objEnd, objContent, err := findObjectBoundaries(pdfBytes, streamObjIndex, streamObjNum, "stream", verbose)
	if err != nil {
		return nil, 0, err
	}

	if verbose {
		log.Printf("Stream object %d: objStart=%d, objEnd=%d, content length=%d", streamObjNum, objStart, objEnd, len(objContent))
	}

	// Find structure markers (these are NOT encrypted)
	objKeywordPosInContent := bytes.Index(objContent, []byte("obj"))
	if objKeywordPosInContent == -1 {
		return nil, 0, fmt.Errorf("'obj' keyword not found in stream object %d", streamObjNum)
	}

	// Extract generation number from object header (CRITICAL for correct key derivation)
	genNum := extractGenerationNumber(objContent, streamObjNum)
	if verbose {
		log.Printf("Extracted generation number for object %d: %d", streamObjNum, genNum)
	}

	// Skip "obj" and whitespace to find where encrypted data starts
	dataStart := objKeywordPosInContent + 3
	for dataStart < len(objContent) && (objContent[dataStart] == ' ' || objContent[dataStart] == '\r' || objContent[dataStart] == '\n' || objContent[dataStart] == '\t') {
		dataStart++
	}

	// Find "stream" keyword (not encrypted)
	streamKeywordPos := bytes.Index(objContent, []byte("stream"))
	if streamKeywordPos == -1 {
		return nil, 0, fmt.Errorf("stream keyword not found for object %d", streamObjNum)
	}

	// Get /Length from dictionary (between dataStart and streamKeywordPos)
	dictContent := string(objContent[dataStart:streamKeywordPos])
	lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
	lengthMatch := lengthPattern.FindStringSubmatch(dictContent)

	var streamLength int
	if lengthMatch != nil {
		streamLength, _ = strconv.Atoi(lengthMatch[1])
	}

	// Extract stream data
	// Skip "stream" keyword (6 bytes) and EOL
	streamDataStart := streamKeywordPos + 6

	// Skip exactly one EOL marker per PDF spec
	if streamDataStart < len(objContent) && objContent[streamDataStart] == '\r' {
		streamDataStart++
	}
	if streamDataStart < len(objContent) && objContent[streamDataStart] == '\n' {
		streamDataStart++
	}

	// Use /Length if available, otherwise find endstream
	var streamContent []byte
	if streamLength > 0 && streamDataStart+streamLength <= len(objContent) {
		streamContent = objContent[streamDataStart : streamDataStart+streamLength]
	} else {
		// Fallback: find endstream
		endstreamPos := bytes.Index(objContent[streamDataStart:], []byte("endstream"))
		if endstreamPos == -1 {
			return nil, 0, fmt.Errorf("endstream not found for object %d", streamObjNum)
		}
		streamContent = objContent[streamDataStart : streamDataStart+endstreamPos]
	}

	streamDataEnd := streamDataStart + len(streamContent)

	if verbose {
		log.Printf("Stream object %d: dictionary encrypted from %d to %d, stream data from %d to %d",
			streamObjNum, dataStart, streamKeywordPos, streamDataStart, streamDataEnd)
	}

	// Decrypt stream data if needed (dictionary decryption not needed for extraction, only for reading)
	if encryptInfo != nil {
		// Decrypt the stream data itself (not the dictionary)
		// Use the extracted generation number instead of hardcoded 0
		decryptedStream, err := encryption.DecryptObject(streamContent, streamObjNum, genNum, encryptInfo)
		if err != nil {
			return nil, 0, fmt.Errorf("error decrypting stream data for object %d (gen %d): %v", streamObjNum, genNum, err)
		}
		streamContent = decryptedStream
		if verbose {
			log.Printf("Decrypted stream data: %d bytes -> %d bytes", len(objContent[streamDataStart:streamDataEnd]), len(streamContent))
		}
	}

	return streamContent, streamObjNum, nil
}

// DecompressStream attempts to decompress a stream (handles FlateDecode)
// FlateDecode can be either raw deflate or zlib-wrapped
func DecompressStream(streamBytes []byte) ([]byte, bool, error) {
	// Try zlib decompression first (FlateDecode is usually zlib-wrapped)
	zlibReader, zlibErr := zlib.NewReader(bytes.NewReader(streamBytes))
	if zlibErr == nil {
		decompressed, err := io.ReadAll(zlibReader)
		zlibReader.Close()
		if err == nil {
			return decompressed, true, nil
		}
		// zlib read failed, try raw deflate
	}

	// Try raw deflate
	reader := flate.NewReader(bytes.NewReader(streamBytes))
	decompressed, err := io.ReadAll(reader)
	reader.Close()

	if err == nil {
		// Successfully decompressed
		return decompressed, true, nil
	}

	// Not compressed or different compression - return as-is
	return streamBytes, false, nil
}

// CompressStream compresses data using FlateDecode
func CompressStream(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}

	_, err = writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ReplaceStreamInPDF replaces a stream in the PDF and updates the length
func ReplaceStreamInPDF(pdfBytes []byte, streamObjNum int, newStream []byte, verbose bool) ([]byte, error) {
	pdfStr := string(pdfBytes)

	// Find the stream object
	streamObjPattern := regexp.MustCompile(fmt.Sprintf(`%d\s+%d\s+obj`, streamObjNum, 0))
	streamObjMatch := streamObjPattern.FindStringIndex(pdfStr)
	if streamObjMatch == nil {
		return nil, fmt.Errorf("stream object %d not found", streamObjNum)
	}

	// Find stream dictionary (before "stream" keyword)
	streamKeywordPos := bytes.Index(pdfBytes[streamObjMatch[0]:], []byte("stream"))
	if streamKeywordPos == -1 {
		return nil, fmt.Errorf("stream keyword not found")
	}

	dictStart := streamObjMatch[0]
	dictEnd := streamObjMatch[0] + streamKeywordPos

	// Find and update /Length entry
	lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
	lengthMatch := lengthPattern.FindStringSubmatchIndex(string(pdfBytes[dictStart:dictEnd]))
	if lengthMatch == nil {
		return nil, fmt.Errorf("Length entry not found in stream dictionary")
	}

	lengthStart := dictStart + lengthMatch[2]
	lengthEnd := dictStart + lengthMatch[3]

	// Replace length
	newLength := strconv.Itoa(len(newStream))

	// Find stream start and end (need to recalculate after length update)
	// Re-find stream position - use bytes.Index since we'll be modifying pdfBytes
	streamKeywordPos = bytes.Index(pdfBytes[dictStart:], []byte("stream"))
	if streamKeywordPos == -1 {
		return nil, fmt.Errorf("stream keyword not found")
	}

	// Skip "stream" keyword (6 bytes) and any whitespace
	streamStart := dictStart + streamKeywordPos + 6
	for streamStart < len(pdfBytes) && (pdfBytes[streamStart] == '\r' || pdfBytes[streamStart] == '\n' || pdfBytes[streamStart] == ' ' || pdfBytes[streamStart] == '\t') {
		streamStart++
	}

	endstreamPos := bytes.Index(pdfBytes[streamStart:], []byte("endstream"))
	if endstreamPos == -1 {
		return nil, fmt.Errorf("endstream not found")
	}

	streamEnd := streamStart + endstreamPos
	afterStream := pdfBytes[streamEnd:]

	// Reconstruct PDF: before length + new length + between length and stream + new stream + after stream
	beforeLength := pdfBytes[:lengthStart]
	betweenLengthAndStream := pdfBytes[lengthEnd:streamStart]

	result := append(beforeLength, []byte(newLength)...)
	result = append(result, betweenLengthAndStream...)
	result = append(result, newStream...)
	result = append(result, afterStream...)

	if verbose {
		log.Printf("Replaced stream %d: old length %s, new length %d", streamObjNum, string(pdfBytes[lengthStart:lengthEnd]), len(newStream))
	}

	return result, nil
}
