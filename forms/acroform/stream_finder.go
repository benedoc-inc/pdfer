// Package acroform provides utilities to find object streams
package acroform

import (
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// findStreamForObject finds which object stream contains a given object
func findStreamForObject(pdfBytes []byte, objNum int, encryptInfo *types.PDFEncryption, verbose bool) int {
	// Parse xref to find object stream info
	startXRef := findStartXRef(pdfBytes)
	if startXRef < 0 {
		return 0
	}

	xrefResult, err := parse.ParseXRefStreamFull(pdfBytes, int64(startXRef), false)
	if err != nil {
		return 0
	}

	// Check if object is in a stream
	if entry, ok := xrefResult.ObjectStreams[objNum]; ok {
		return entry.StreamObjNum
	}

	return 0
}

// findStartXRef finds the startxref position
func findStartXRef(pdfBytes []byte) int64 {
	// Search from end
	searchLen := 1024
	if len(pdfBytes) < searchLen {
		searchLen = len(pdfBytes)
	}

	tail := pdfBytes[len(pdfBytes)-searchLen:]
	pattern := regexp.MustCompile(`startxref\s+(\d+)`)
	match := pattern.FindSubmatch(tail)
	if match != nil {
		val, err := strconv.ParseInt(string(match[1]), 10, 64)
		if err == nil {
			return val
		}
	}
	return -1
}
