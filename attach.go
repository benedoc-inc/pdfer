package pdfer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/incremental"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
)

// FileAttachment holds a file to be embedded in the exported PDF.
type FileAttachment struct {
	Name     string // filename to display in the PDF attachment panel
	Data     []byte // raw file contents
	MimeType string // MIME type, e.g. "application/pdf" or "image/jpeg"
}

// EmbedAttachments appends file attachments to pdfBytes via an incremental
// update. The attached files appear in PDF viewers' attachment panels.
//
// For encrypted PDFs, password must be the user or owner password; the appended
// embedded-file streams and filespec strings are encrypted with the document's
// keys and the file /ID is carried forward so the update remains decryptable.
// Pass a nil password for unencrypted documents.
func EmbedAttachments(pdfBytes []byte, files []FileAttachment, password []byte) ([]byte, error) {
	if len(files) == 0 {
		return pdfBytes, nil
	}

	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password})
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("no trailer found")
	}

	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("bad Root ref %q: %w", trailer.RootRef, err)
	}

	catalogBody, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	upd, err := incremental.New(pdfBytes, trailer, pdf.Encryption())
	if err != nil {
		return nil, err
	}

	// One embedded-file stream object + one filespec dict object per file.
	type attachment struct {
		name       string
		filespecNo int
	}
	attachments := make([]attachment, 0, len(files))

	for _, f := range files {
		mimeType := f.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		streamNo := upd.Reserve()
		dict := fmt.Sprintf(
			"<</Type /EmbeddedFile /Subtype /%s /Filter /FlateDecode>>",
			pdfNameEscape(mimeType),
		)
		if err := upd.AddStream(streamNo, 0, []byte(dict), compressBytes(f.Data)); err != nil {
			return nil, err
		}

		filespecNo := upd.Reserve()
		escapedName := pdfStringEscape(f.Name)
		filespec := fmt.Sprintf(
			"<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>",
			escapedName, escapedName, streamNo,
		)
		if err := upd.AddObject(filespecNo, 0, []byte(filespec)); err != nil {
			return nil, err
		}

		attachments = append(attachments, attachment{name: f.Name, filespecNo: filespecNo})
	}

	// /Names object: EmbeddedFiles name tree (flat, no kids).
	namesNo := upd.Reserve()
	var names bytes.Buffer
	names.WriteString("<</EmbeddedFiles <</Names [")
	for _, a := range attachments {
		fmt.Fprintf(&names, " (%s) %d 0 R", pdfStringEscape(a.name), a.filespecNo)
	}
	names.WriteString(" ]>>>>")
	if err := upd.AddObject(namesNo, 0, names.Bytes()); err != nil {
		return nil, err
	}

	// Updated catalog: copy old body, add/replace /Names entry.
	if err := upd.AddObject(rootNum, 0, injectNamesRef(catalogBody, namesNo)); err != nil {
		return nil, err
	}

	return upd.Bytes()
}

// injectNamesRef returns a copy of catalogBody with /Names N 0 R added or
// replaced. It strips any pre-existing top-level /Names entry and appends
// a fresh one before the closing >>.
func injectNamesRef(catalogBody []byte, namesNo int) []byte {
	s := string(catalogBody)
	namesPat := regexp.MustCompile(`/Names\s+\d+\s+\d+\s+R`)
	s = namesPat.ReplaceAllString(s, "")
	end := strings.LastIndex(s, ">>")
	if end == -1 {
		return []byte(fmt.Sprintf("<</Names %d 0 R>>", namesNo))
	}
	newRef := fmt.Sprintf("/Names %d 0 R", namesNo)
	return []byte(s[:end] + newRef + s[end:])
}

func compressBytes(data []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(data)
	zw.Close()
	return buf.Bytes()
}

// pdfNameEscape converts a MIME type like "application/pdf" to a PDF name
// fragment suitable for /Subtype. Slashes become #2F.
func pdfNameEscape(s string) string {
	return strings.ReplaceAll(s, "/", "#2F")
}

// pdfStringEscape escapes a string for use inside PDF string literals ().
func pdfStringEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}

func parseRefNum(ref string) (int, error) {
	parts := strings.Fields(ref)
	if len(parts) < 1 {
		return 0, fmt.Errorf("empty ref")
	}
	return strconv.Atoi(parts[0])
}
