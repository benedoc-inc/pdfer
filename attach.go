package pdfer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
)

// FileAttachment holds a file to be embedded in the exported PDF.
type FileAttachment struct {
	Name     string // filename to display in the PDF attachment panel
	Data     []byte // raw file contents
	MimeType string // MIME type, e.g. "application/pdf" or "image/jpeg"
}

// EmbedAttachments appends file attachments to pdfBytes via an incremental
// update. The attached files appear in PDF viewers' attachment panels.
func EmbedAttachments(pdfBytes []byte, files []FileAttachment) ([]byte, error) {
	if len(files) == 0 {
		return pdfBytes, nil
	}

	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{})
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("no trailer found")
	}

	prevXRef := attachFindStartXRef(pdfBytes)
	if prevXRef < 0 {
		return nil, fmt.Errorf("could not locate startxref")
	}

	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("bad Root ref %q: %w", trailer.RootRef, err)
	}

	catalogBody, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	nextObj := trailer.Size
	offsets := make(map[int]int64)

	var buf bytes.Buffer
	buf.Write(pdfBytes)
	buf.WriteByte('\n')

	// One embedded-file stream object + one filespec dict object per file.
	type attachment struct {
		name       string
		filespecNo int
	}
	attachments := make([]attachment, 0, len(files))

	for _, f := range files {
		compressed := compressBytes(f.Data)
		mimeType := f.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		streamNo := nextObj
		nextObj++
		offsets[streamNo] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", streamNo)
		fmt.Fprintf(&buf,
			"<</Type /EmbeddedFile /Subtype /%s /Filter /FlateDecode /Length %d>>\n",
			pdfNameEscape(mimeType), len(compressed),
		)
		buf.WriteString("stream\n")
		buf.Write(compressed)
		buf.WriteString("\nendstream\nendobj\n")

		filespecNo := nextObj
		nextObj++
		offsets[filespecNo] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", filespecNo)
		escapedName := pdfStringEscape(f.Name)
		fmt.Fprintf(&buf,
			"<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>\n",
			escapedName, escapedName, streamNo,
		)
		buf.WriteString("endobj\n")

		attachments = append(attachments, attachment{name: f.Name, filespecNo: filespecNo})
	}

	// /Names object: EmbeddedFiles name tree (flat, no kids).
	namesNo := nextObj
	nextObj++
	offsets[namesNo] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n", namesNo)
	buf.WriteString("<</EmbeddedFiles <</Names [")
	for _, a := range attachments {
		fmt.Fprintf(&buf, " (%s) %d 0 R", pdfStringEscape(a.name), a.filespecNo)
	}
	buf.WriteString(" ]>>>>\nendobj\n")

	// Updated catalog: copy old body, add/replace /Names entry.
	newCatalog := injectNamesRef(catalogBody, namesNo)
	offsets[rootNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n", rootNum)
	buf.Write(newCatalog)
	buf.WriteString("\nendobj\n")

	// xref table.
	xrefStart := int64(buf.Len())
	buf.WriteString("xref\n")
	nums := make([]int, 0, len(offsets))
	for n := range offsets {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	attachWriteXRef(&buf, offsets, nums)

	// Trailer.
	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, "/Size %d", nextObj)
	fmt.Fprintf(&buf, "/Root %s", trailer.RootRef)
	if trailer.InfoRef != "" {
		fmt.Fprintf(&buf, "/Info %s", trailer.InfoRef)
	}
	if trailer.EncryptRef != "" {
		fmt.Fprintf(&buf, "/Encrypt %s", trailer.EncryptRef)
	}
	fmt.Fprintf(&buf, "/Prev %d", prevXRef)
	buf.WriteString(">>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefStart)

	return buf.Bytes(), nil
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

func attachFindStartXRef(pdfBytes []byte) int64 {
	searchLen := 1024
	if searchLen > len(pdfBytes) {
		searchLen = len(pdfBytes)
	}
	tail := pdfBytes[len(pdfBytes)-searchLen:]
	pat := regexp.MustCompile(`startxref\s+(\d+)`)
	// Take the last match.
	matches := pat.FindAllSubmatch(tail, -1)
	if len(matches) == 0 {
		return -1
	}
	last := matches[len(matches)-1]
	v, err := strconv.ParseInt(string(last[1]), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

func attachWriteXRef(buf *bytes.Buffer, offsets map[int]int64, nums []int) {
	i := 0
	for i < len(nums) {
		j := i + 1
		for j < len(nums) && nums[j] == nums[j-1]+1 {
			j++
		}
		fmt.Fprintf(buf, "%d %d\n", nums[i], j-i)
		for k := i; k < j; k++ {
			fmt.Fprintf(buf, "%010d %05d n \n", offsets[nums[k]], 0)
		}
		i = j
	}
}
