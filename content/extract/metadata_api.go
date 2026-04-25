package extract

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// GetMetadata parses pdfBytes and returns document metadata without requiring
// a pre-parsed *parse.PDF.
func GetMetadata(pdfBytes []byte, password []byte, verbose bool) (*types.DocumentMetadata, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password, Verbose: verbose})
	if err != nil {
		return nil, fmt.Errorf("get metadata: parse failed: %w", err)
	}
	return ExtractMetadata(pdfBytes, pdf, verbose)
}

// GetBookmarks parses pdfBytes and returns the document's bookmark/outline
// tree without requiring a pre-parsed *parse.PDF.
func GetBookmarks(pdfBytes []byte, password []byte, verbose bool) ([]types.Bookmark, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password, Verbose: verbose})
	if err != nil {
		return nil, fmt.Errorf("get bookmarks: parse failed: %w", err)
	}
	return ExtractBookmarks(pdfBytes, pdf, verbose)
}
