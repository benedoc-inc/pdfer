package xfa

import (
	"fmt"
	"log"

	"github.com/benedoc-inc/pdfer/v2/core/incremental"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// UpdateXFAInPDFIncremental updates XFA datasets field values by appending a
// PDF incremental update (ISO 32000-1 §7.5.6) instead of splicing bytes in
// place. The original bytes are preserved verbatim as a prefix of the output:
// no offsets shift, the source's encryption (if any) is preserved — the
// rewritten datasets stream is re-encrypted with the document key — and
// signatures covering the original revision are not invalidated.
//
// This is the only fill path that works for sources whose active xref is a
// cross-reference stream (PDF 1.5+, e.g. FDA eSTAR templates), which
// ReplaceStreamInPDF rejects with ErrXRefStreamUnsupported. The appended
// cross-reference section mirrors the source's form (table or stream).
func UpdateXFAInPDFIncremental(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("error parsing PDF: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}

	xfaXML, streamObjNum, err := FindXFADatasetsStream(pdfBytes, pdf.Encryption(), verbose)
	if err != nil {
		return nil, fmt.Errorf("error finding XFA datasets stream: %w", err)
	}
	if verbose {
		log.Printf("Incremental XFA fill: datasets stream at object %d (%d bytes)", streamObjNum, len(xfaXML))
	}

	updatedXML, err := UpdateXFAValues(string(xfaXML), formData, verbose)
	if err != nil {
		return nil, fmt.Errorf("error updating XFA values: %w", err)
	}

	compressed, err := CompressStream([]byte(updatedXML))
	if err != nil {
		return nil, fmt.Errorf("error compressing datasets stream: %w", err)
	}

	upd, err := incremental.New(pdfBytes, trailer, pdf.Encryption())
	if err != nil {
		return nil, fmt.Errorf("error starting incremental update: %w", err)
	}
	// The object is replaced wholesale, dict and data together, so the
	// replacement always declares (and carries) FlateDecode regardless of how
	// the original stream was filtered. AddStream injects /Length and, for
	// encrypted documents, re-encrypts the data with the per-object key.
	if err := upd.AddStream(streamObjNum, 0, []byte("<</Filter/FlateDecode>>"), compressed); err != nil {
		return nil, fmt.Errorf("error appending datasets stream: %w", err)
	}
	return upd.Bytes()
}
