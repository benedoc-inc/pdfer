package pdfer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/incremental"
	"github.com/benedoc-inc/pdfer/core/parse"
)

// FileAttachment holds a file to be embedded in the exported PDF.
type FileAttachment struct {
	Name     string // filename to display in the PDF attachment panel
	Data     []byte // raw file contents
	MimeType string // MIME type, e.g. "application/pdf" or "image/jpeg"
}

// EmbedAttachments appends file attachments to an unencrypted pdfBytes via an
// incremental update. The attached files appear in PDF viewers' attachment
// panels. For encrypted PDFs use EmbedAttachmentsWithPassword.
func EmbedAttachments(pdfBytes []byte, files []FileAttachment) ([]byte, error) {
	return EmbedAttachmentsWithPassword(pdfBytes, files, nil)
}

// EmbedAttachmentsWithPassword appends file attachments to pdfBytes via an
// incremental update, supplying the user or owner password for encrypted
// documents (pass nil for unencrypted ones).
//
// For encrypted PDFs the appended embedded-file streams are encrypted with the
// document's keys, string values follow the document's /StrF crypt filter, and
// the file /ID is carried forward so the update remains decryptable.
func EmbedAttachmentsWithPassword(pdfBytes []byte, files []FileAttachment, password []byte) ([]byte, error) {
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

// ListAttachments returns the embedded files reachable from the PDF's
// /EmbeddedFiles name tree (the catalog's /Names entry). Nested name-tree
// nodes (/Kids) are traversed. Each returned FileAttachment carries the
// decoded (decompressed, and for encrypted PDFs decrypted) file contents, so
// the result round-trips back through EmbedAttachments.
//
// Only the /EmbeddedFiles name tree is consulted. Files attached solely via
// /FileAttachment annotations or /AF associated-file arrays are not (yet)
// reported.
//
// For encrypted PDFs use ListAttachmentsWithPassword.
func ListAttachments(pdfBytes []byte) ([]FileAttachment, error) {
	return ListAttachmentsWithPassword(pdfBytes, nil)
}

// ListAttachmentsWithPassword is ListAttachments for encrypted documents,
// supplying the user or owner password (pass nil for unencrypted ones). The
// returned file contents are decrypted with the document's keys.
func ListAttachmentsWithPassword(pdfBytes []byte, password []byte) ([]FileAttachment, error) {
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
	catalog, err := pdf.GetObject(rootNum)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	// catalog -> /Names dict -> /EmbeddedFiles -> name-tree root node.
	namesDict, err := attachResolveSubDict(pdf, catalog, "Names")
	if err != nil || namesDict == nil {
		// No /Names entry at all: the document has no embedded-file name tree.
		return nil, nil
	}
	rootNode, err := attachResolveSubDict(pdf, namesDict, "EmbeddedFiles")
	if err != nil || rootNode == nil {
		return nil, nil
	}

	var out []FileAttachment
	if err := attachWalkNameTree(pdf, rootNode, &out, 0); err != nil {
		return nil, err
	}
	return out, nil
}

// attachWalkNameTree recurses through an EmbeddedFiles name-tree node,
// collecting one FileAttachment per filespec found in any leaf /Names array.
// depth guards against cyclic or pathological /Kids chains.
func attachWalkNameTree(pdf *parse.PDF, node []byte, out *[]FileAttachment, depth int) error {
	if depth > 64 {
		return fmt.Errorf("name tree nested too deeply")
	}
	// Intermediate node: recurse into each kid.
	if kids := attachFindArray(node, "Kids"); kids != nil {
		for _, ref := range attachArrayRefs(kids) {
			kidBody, err := pdf.GetObject(ref)
			if err != nil {
				continue
			}
			if err := attachWalkNameTree(pdf, kidBody, out, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	// Leaf node: every reference in the /Names array is a filespec. The
	// interleaved string keys are display names; the authoritative filename
	// lives on the filespec's /F or /UF entry, so we read those instead.
	names := attachFindArray(node, "Names")
	if names == nil {
		return nil
	}
	for _, ref := range attachArrayRefs(names) {
		att, ok := attachReadFilespec(pdf, ref)
		if ok {
			*out = append(*out, att)
		}
	}
	return nil
}

// attachReadFilespec resolves a /Filespec object and its embedded-file stream
// into a FileAttachment. ok is false when the filespec carries no embedded
// file (e.g. an external-file reference) or cannot be read.
func attachReadFilespec(pdf *parse.PDF, filespecNum int) (FileAttachment, bool) {
	body, err := pdf.GetObject(filespecNum)
	if err != nil {
		return FileAttachment{}, false
	}

	name := attachFindString(body, "UF")
	if name == "" {
		name = attachFindString(body, "F")
	}

	// /EF <</F n 0 R ...>> points at the EmbeddedFile stream. Prefer /F, then
	// /UF; otherwise take the first reference in the dict.
	ef := attachFindSubDict(body, "EF")
	if ef == nil {
		return FileAttachment{}, false
	}
	streamNum := attachFindRef(ef, "F")
	if streamNum < 0 {
		streamNum = attachFindRef(ef, "UF")
	}
	if streamNum < 0 {
		refs := attachArrayRefs(ef)
		if len(refs) == 0 {
			return FileAttachment{}, false
		}
		streamNum = refs[0]
	}

	streamObj, err := pdf.GetObject(streamNum)
	if err != nil {
		return FileAttachment{}, false
	}
	data, err := attachDecodeStream(pdf, streamObj)
	if err != nil {
		return FileAttachment{}, false
	}

	mime := ""
	if sub := attachFindName(streamObj, "Subtype"); sub != "" {
		mime = pdfNameUnescape(sub)
	}
	return FileAttachment{Name: name, Data: data, MimeType: mime}, true
}

// attachDecodeStream slices the raw stream bytes out of an object and applies
// its /Filter chain. /Length is honoured when present (resolving an indirect
// length reference if needed); otherwise the data runs to "endstream".
func attachDecodeStream(pdf *parse.PDF, obj []byte) ([]byte, error) {
	// Bound the dictionary first: searching for "stream" naively would match
	// inside names like /Subtype /application#2Foctet-stream. Match the dict's
	// own << >> and only look for the stream keyword past its close.
	dictStart := bytes.Index(obj, []byte("<<"))
	if dictStart == -1 {
		return nil, fmt.Errorf("no stream dict")
	}
	dictEnd := attachMatchDelimited(obj, dictStart, '<', '>')
	if dictEnd == -1 {
		return nil, fmt.Errorf("unterminated stream dict")
	}
	dict := obj[dictStart : dictEnd+1]

	rel := bytes.Index(obj[dictEnd:], []byte("stream"))
	if rel == -1 {
		return nil, fmt.Errorf("no stream keyword")
	}
	sIdx := dictEnd + rel

	start := sIdx + len("stream")
	if start < len(obj) && obj[start] == '\r' {
		start++
	}
	if start < len(obj) && obj[start] == '\n' {
		start++
	}

	// Determine the raw stream length.
	var raw []byte
	if n, ok := attachResolveLength(pdf, dict); ok && start+n <= len(obj) {
		raw = obj[start : start+n]
	} else if eIdx := bytes.Index(obj[start:], []byte("endstream")); eIdx != -1 {
		raw = obj[start : start+eIdx]
		// Trim the single EOL that precedes "endstream", if present.
		raw = bytes.TrimRight(raw, "\r\n")
	} else {
		return nil, fmt.Errorf("no endstream")
	}

	filters := attachFilterChain(dict)
	decoded := raw
	for _, f := range filters {
		d, err := parse.DecodeFilter(decoded, f)
		if err != nil {
			return nil, fmt.Errorf("decode filter %s: %w", f, err)
		}
		decoded = d
	}
	return decoded, nil
}

// attachResolveLength reads /Length from a stream dict, following an indirect
// reference when the length is given as "N G R".
func attachResolveLength(pdf *parse.PDF, dict []byte) (int, bool) {
	if m := regexp.MustCompile(`/Length\s+(\d+)\s+\d+\s+R`).FindSubmatch(dict); m != nil {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return 0, false
		}
		lenObj, err := pdf.GetObjectContent(n)
		if err != nil {
			return 0, false
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(lenObj)))
		if err != nil {
			return 0, false
		}
		return v, true
	}
	if m := regexp.MustCompile(`/Length\s+(\d+)`).FindSubmatch(dict); m != nil {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// attachFilterChain returns the ordered list of filter names from /Filter,
// accepting either a single name (/FlateDecode) or an array ([/A85 /Fl]).
// An empty result means the stream is stored uncompressed.
func attachFilterChain(dict []byte) []string {
	if arr := attachFindArray(dict, "Filter"); arr != nil {
		return regexp.MustCompile(`/(\w+)`).FindAllString(string(arr), -1)
	}
	if m := regexp.MustCompile(`/Filter\s*(/\w+)`).FindSubmatch(dict); m != nil {
		return []string{string(m[1])}
	}
	return nil
}

// --- name-tree / dictionary scanning helpers --------------------------------

// attachResolveSubDict returns the dictionary value following /key in dict,
// whether it is written inline (<<...>>) or as an indirect reference.
func attachResolveSubDict(pdf *parse.PDF, dict []byte, key string) ([]byte, error) {
	if sub := attachFindSubDict(dict, key); sub != nil {
		return sub, nil
	}
	if ref := attachFindRef(dict, key); ref >= 0 {
		return pdf.GetObject(ref)
	}
	return nil, nil
}

// attachFindRef returns the object number in `/key N G R`, or -1 if absent.
func attachFindRef(dict []byte, key string) int {
	m := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s+(\d+)\s+\d+\s+R`).FindSubmatch(dict)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return -1
	}
	return n
}

// attachFindName returns the name value (without the leading slash) following
// /key, e.g. attachFindName(d, "Subtype") == "application#2Fpdf".
func attachFindName(dict []byte, key string) string {
	m := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s*/([^\s/<>\[\]()]+)`).FindSubmatch(dict)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// attachFindString returns the unescaped literal string following /key, e.g.
// the (filename) of /F or /UF. Only PDF literal strings () are handled.
func attachFindString(dict []byte, key string) string {
	idx := attachKeyValueStart(dict, key)
	if idx < 0 || idx >= len(dict) || dict[idx] != '(' {
		return ""
	}
	// Scan to the matching ) honouring backslash escapes and nested literals.
	depth := 0
	for i := idx; i < len(dict); i++ {
		switch dict[i] {
		case '\\':
			i++ // skip the escaped byte
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pdfStringUnescape(dict[idx+1 : i])
			}
		}
	}
	return ""
}

// attachFindSubDict returns the bytes between (and excluding) the << >> of the
// dictionary value following /key, or nil if the value is not an inline dict.
func attachFindSubDict(dict []byte, key string) []byte {
	idx := attachKeyValueStart(dict, key)
	if idx < 0 || idx+1 >= len(dict) || dict[idx] != '<' || dict[idx+1] != '<' {
		return nil
	}
	if end := attachMatchDelimited(dict, idx, '<', '>'); end > idx {
		return dict[idx+2 : end-1]
	}
	return nil
}

// attachFindArray returns the bytes between (and excluding) the [ ] of the
// array value following /key, or nil if the value is not an inline array.
func attachFindArray(dict []byte, key string) []byte {
	idx := attachKeyValueStart(dict, key)
	if idx < 0 || idx >= len(dict) || dict[idx] != '[' {
		return nil
	}
	if end := attachMatchDelimited(dict, idx, '[', ']'); end > idx {
		return dict[idx+1 : end]
	}
	return nil
}

// attachArrayRefs returns every object number appearing as "N G R" in arr.
func attachArrayRefs(arr []byte) []int {
	ms := regexp.MustCompile(`(\d+)\s+\d+\s+R`).FindAllSubmatch(arr, -1)
	refs := make([]int, 0, len(ms))
	for _, m := range ms {
		if n, err := strconv.Atoi(string(m[1])); err == nil {
			refs = append(refs, n)
		}
	}
	return refs
}

// attachKeyValueStart returns the index of the first non-whitespace byte of
// the value following the literal name /key in dict, or -1 if key is absent.
func attachKeyValueStart(dict []byte, key string) int {
	loc := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\b`).FindIndex(dict)
	if loc == nil {
		return -1
	}
	i := loc[1]
	for i < len(dict) && (dict[i] == ' ' || dict[i] == '\t' || dict[i] == '\r' || dict[i] == '\n') {
		i++
	}
	if i >= len(dict) {
		return -1
	}
	return i
}

// attachMatchDelimited scans from an opening delimiter at start and returns
// the index of the matching close delimiter, tracking nesting depth. PDF
// literal strings (...) are skipped (honouring backslash escapes) so that
// delimiters appearing inside strings do not throw off the balance. For the
// << >> case, pass open='<' close='>': the doubled angle brackets nest
// consistently, so depth tracking still pairs them correctly.
func attachMatchDelimited(data []byte, start int, open, close byte) int {
	depth := 0
	inString := false
	for i := start; i < len(data); i++ {
		c := data[i]
		if inString {
			switch c {
			case '\\':
				i++ // skip escaped byte
			case ')':
				inString = false
			}
			continue
		}
		switch c {
		case '(':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// pdfStringUnescape reverses pdfStringEscape for the common escapes produced
// by this package and standard writers: \\ \( \) and \r \n \t.
func pdfStringUnescape(s []byte) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// pdfNameUnescape reverses pdfNameEscape, decoding #XX hex sequences in a PDF
// name back to their literal bytes (e.g. "application#2Fpdf" -> "application/pdf").
func pdfNameUnescape(s string) string {
	if !strings.Contains(s, "#") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
