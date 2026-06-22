package xfa

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/v2/core/incremental"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
)

// formChecksumAttrRE matches the checksum attribute on the root <form> element.
// [^>] (not . ) is used so it spans the newlines Adobe's split-tag pretty printing
// inserts between attributes (e.g. <form\n checksum="..."\n xmlns="...">).
var formChecksumAttrRE = regexp.MustCompile(`(<form\b[^>]*?\bchecksum=")[^"]*(")`)

// FormChecksumFunc computes the XFA <form checksum="..."> digest over a PDF's
// template and datasets packets. pdfer deliberately does not include a digest
// algorithm; a caller that needs a real, Acrobat-honored checksum supplies its
// own implementation here. A nil func skips stamping entirely, leaving whatever
// checksum the form packet already carries.
type FormChecksumFunc func(template, datasets []byte) (string, error)

// SetXFAFormPacket replaces the XFA `form` packet of an eSTAR with formPacket so
// Acrobat restores the packet's saved form-state deltas (revealed pages, swapped
// instruction text, instance counts) on open instead of discarding them.
//
// The replacement is applied as a PDF incremental update (the original bytes are
// preserved verbatim as a prefix; encryption and signatures over the original
// revision are kept), which is the only fill path that works for the
// cross-reference-stream PDFs eSTARs are.
//
// When checksum is non-nil it is invoked over the PDF's CURRENT template +
// datasets packets and the result is stamped onto the form packet's <form>
// element; Acrobat discards the saved deltas unless this digest matches what it
// recomputes at open time, so callers that also change the datasets must write
// that first so the stamped digest stays valid. When checksum is nil the form
// packet is inserted as-is (its existing checksum, if any, is preserved).
//
// The target eSTAR must already carry a `form` packet in its /XFA array (the
// FDA-distributed forms do). Adding a brand-new `form` entry to the array is not
// yet supported and returns an error.
func SetXFAFormPacket(pdfBytes, formPacket, password []byte, verbose bool, checksum FormChecksumFunc) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password, Verbose: verbose})
	if err != nil {
		return nil, fmt.Errorf("parsing PDF: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}
	enc := pdf.Encryption()

	streams, err := ExtractAllXFAStreams(pdfBytes, enc, verbose)
	if err != nil {
		return nil, fmt.Errorf("extracting XFA streams: %w", err)
	}
	if streams.Template == nil || streams.Datasets == nil {
		return nil, fmt.Errorf("PDF is missing XFA template or datasets packet")
	}
	if streams.Form == nil {
		return nil, fmt.Errorf("PDF has no XFA form packet to replace (adding one is not yet supported)")
	}
	formInfo := streams.Form

	if checksum != nil {
		sum, err := checksum(streams.Template.Data, streams.Datasets.Data)
		if err != nil {
			return nil, fmt.Errorf("computing form checksum: %w", err)
		}
		formPacket, err = stampFormChecksum(formPacket, sum)
		if err != nil {
			return nil, err
		}
	}

	compressed, err := CompressStream(formPacket)
	if err != nil {
		return nil, fmt.Errorf("compressing form packet: %w", err)
	}

	upd, err := incremental.New(pdfBytes, trailer, enc)
	if err != nil {
		return nil, fmt.Errorf("starting incremental update: %w", err)
	}
	// Replace the form packet object wholesale; the replacement declares
	// FlateDecode and AddStream re-encrypts it with the document key when the
	// source is encrypted (eSTARs are).
	if err := upd.AddStream(formInfo.ObjectNumber, 0, []byte("<</Filter/FlateDecode>>"), compressed); err != nil {
		return nil, fmt.Errorf("appending form packet stream: %w", err)
	}
	return upd.Bytes()
}

// stampFormChecksum sets the checksum attribute on the form packet's root <form>
// element to checksum, replacing any existing value or inserting the attribute
// when none is present.
func stampFormChecksum(formPacket []byte, checksum string) ([]byte, error) {
	if formChecksumAttrRE.Match(formPacket) {
		return formChecksumAttrRE.ReplaceAll(formPacket, []byte("${1}"+checksum+"${2}")), nil
	}
	idx := bytes.Index(formPacket, []byte("<form"))
	if idx < 0 {
		return nil, fmt.Errorf("form packet has no <form> root element")
	}
	at := idx + len("<form")
	out := make([]byte, 0, len(formPacket)+len(checksum)+16)
	out = append(out, formPacket[:at]...)
	out = append(out, ` checksum="`...)
	out = append(out, checksum...)
	out = append(out, '"')
	out = append(out, formPacket[at:]...)
	return out, nil
}
