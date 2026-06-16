package pdfer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"sort"
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
// Attachments already embedded in the document are preserved: the existing
// /EmbeddedFiles name tree is merged with the new entries (flattened into a
// single sorted leaf node), and sibling name trees under the catalog's /Names
// dictionary (/Dests, /JavaScript, ...) are carried over. When a new file's
// name collides with an existing entry (or another file in the same batch) it
// is renamed with a -1, -2, ... suffix before the extension.
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

	// Existing /Names dictionary (inline or indirect): its /EmbeddedFiles tree
	// is merged with the new entries, and its other name trees are preserved.
	entries, siblings := attachExistingNames(pdf, catalogBody)
	used := make(map[string]bool, len(entries)+len(files))
	for _, e := range entries {
		used[e.key] = true
	}

	upd, err := incremental.New(pdfBytes, trailer, pdf.Encryption())
	if err != nil {
		return nil, err
	}

	// One embedded-file stream object + one filespec dict object per file.
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
		name := attachUniqueName(f.Name, used)
		escapedName := pdfStringEscape(name)
		filespec := fmt.Sprintf(
			"<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>",
			escapedName, escapedName, streamNo,
		)
		if err := upd.AddObject(filespecNo, 0, []byte(filespec)); err != nil {
			return nil, err
		}

		entries = append(entries, nameTreeEntry{key: name, ref: filespecNo})
	}

	// Name-tree keys must be lexically sorted (ISO 32000-1 §7.9.6). Go string
	// comparison is byte-wise, matching the spec's ordering.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	// /Names object: sibling trees carried over + the merged EmbeddedFiles
	// name tree (flat, no kids).
	namesNo := upd.Reserve()
	var names bytes.Buffer
	names.WriteString("<<")
	names.Write(siblings)
	names.WriteString("/EmbeddedFiles <</Names [")
	for _, e := range entries {
		fmt.Fprintf(&names, " (%s) %d 0 R", pdfStringEscape(e.key), e.ref)
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

// nameTreeEntry is one key/filespec pair of an EmbeddedFiles name tree.
type nameTreeEntry struct {
	key string // decoded tree key (the display name)
	ref int    // filespec object number
}

// attachExistingNames reads the catalog's /Names dictionary and returns the
// entries of its /EmbeddedFiles name tree plus the remaining dictionary body
// (sibling name trees such as /Dests) with the /EmbeddedFiles entry stripped,
// ready to splice into a replacement /Names dictionary. Both the inline-dict
// and indirect-reference forms of /Names are handled.
func attachExistingNames(pdf *parse.PDF, catalogBody []byte) ([]nameTreeEntry, []byte) {
	namesDict, err := attachResolveSubDict(pdf, catalogBody, "Names")
	if err != nil || namesDict == nil {
		return nil, nil
	}

	var entries []nameTreeEntry
	if rootNode, err := attachResolveSubDict(pdf, namesDict, "EmbeddedFiles"); err == nil && rootNode != nil {
		// Best effort: an unreadable subtree keeps the entries found so far.
		_ = attachCollectNameTree(pdf, rootNode, &entries, 0)
	}

	return entries, attachStripDictKey(attachDictInner(namesDict), "EmbeddedFiles")
}

// attachUniqueName returns name if unused, otherwise inserts a -1, -2, ...
// suffix before the extension until the result is unique. The chosen name is
// recorded in used.
func attachUniqueName(name string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := ""
	if dot := strings.LastIndex(name, "."); dot > 0 {
		ext = name[dot:]
	}
	base := name[:len(name)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
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

// attachCollectNameTree recurses through an EmbeddedFiles name-tree node,
// collecting the (key, filespec ref) pairs of every leaf /Names array. Unlike
// attachWalkNameTree it does not resolve the filespecs: the pairs are reused
// as-is when the tree is rewritten, so the existing objects stay referenced.
func attachCollectNameTree(pdf *parse.PDF, node []byte, out *[]nameTreeEntry, depth int) error {
	if depth > 64 {
		return fmt.Errorf("name tree nested too deeply")
	}
	if kids := attachFindArray(node, "Kids"); kids != nil {
		for _, ref := range attachArrayRefs(kids) {
			kidBody, err := pdf.GetObject(ref)
			if err != nil {
				continue
			}
			if err := attachCollectNameTree(pdf, kidBody, out, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if names := attachFindArray(node, "Names"); names != nil {
		*out = append(*out, attachLeafEntries(names)...)
	}
	return nil
}

// attachLeafEntries scans a leaf /Names array, returning each string key
// (literal or hex form) paired with the object number of the filespec
// reference that follows it. Malformed tails are dropped.
func attachLeafEntries(arr []byte) []nameTreeEntry {
	refPat := regexp.MustCompile(`^(\d+)\s+\d+\s+R`)
	var entries []nameTreeEntry
	i := 0
	for i < len(arr) {
		for i < len(arr) && attachIsSpace(arr[i]) {
			i++
		}
		if i >= len(arr) {
			break
		}
		var key string
		switch {
		case arr[i] == '(':
			end := attachLiteralEnd(arr, i)
			if end < 0 {
				return entries
			}
			key = pdfStringUnescape(arr[i+1 : end])
			i = end + 1
		case arr[i] == '<' && (i+1 >= len(arr) || arr[i+1] != '<'):
			end := bytes.IndexByte(arr[i:], '>')
			if end < 0 {
				return entries
			}
			key = attachHexDecode(arr[i+1 : i+end])
			i += end + 1
		default:
			return entries
		}
		for i < len(arr) && attachIsSpace(arr[i]) {
			i++
		}
		m := refPat.FindSubmatch(arr[i:])
		if m == nil {
			return entries
		}
		ref, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return entries
		}
		entries = append(entries, nameTreeEntry{key: key, ref: ref})
		i += len(m[0])
	}
	return entries
}

func attachIsSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}

// attachHexDecode decodes a PDF hex string body (the bytes between < and >).
// Whitespace is skipped and an odd trailing digit is padded with 0, per
// ISO 32000-1 §7.3.4.3.
func attachHexDecode(s []byte) string {
	var b strings.Builder
	hi := byte(0)
	have := false
	for _, c := range s {
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			continue
		}
		if have {
			b.WriteByte(hi<<4 | v)
			have = false
		} else {
			hi = v
			have = true
		}
	}
	if have {
		b.WriteByte(hi << 4)
	}
	return b.String()
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

	// Derive the MIME from the stream dict only, not the whole object: the
	// compressed payload could otherwise contain bytes that look like
	// /Subtype /… and yield a bogus type when the real key is absent.
	mime := ""
	if dict, ok := attachStreamDict(streamObj); ok {
		if sub := attachFindName(dict, "Subtype"); sub != "" {
			mime = pdfNameUnescape(sub)
		}
	}
	return FileAttachment{Name: name, Data: data, MimeType: mime}, true
}

// attachStreamDict returns the bounded << >> dictionary slice of a stream
// object, excluding the raw stream payload. Searching the whole object for
// keys would risk matching bytes inside the compressed stream data.
func attachStreamDict(obj []byte) ([]byte, bool) {
	dictStart := bytes.Index(obj, []byte("<<"))
	if dictStart == -1 {
		return nil, false
	}
	dictEnd := attachMatchDelimited(obj, dictStart, '<', '>')
	if dictEnd == -1 {
		return nil, false
	}
	return obj[dictStart : dictEnd+1], true
}

// attachDecodeStream slices the raw stream bytes out of an object and applies
// its /Filter chain. /Length is honoured when present (resolving an indirect
// length reference if needed); otherwise the data runs to "endstream".
func attachDecodeStream(pdf *parse.PDF, obj []byte) ([]byte, error) {
	// Bound the dictionary first: searching for "stream" naively would match
	// inside names like /Subtype /application#2Foctet-stream. Match the dict's
	// own << >> and only look for the stream keyword past its close.
	dict, ok := attachStreamDict(obj)
	if !ok {
		return nil, fmt.Errorf("no stream dict")
	}
	dictStart := bytes.Index(obj, []byte("<<"))
	dictEnd := dictStart + len(dict) - 1

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
	end := attachLiteralEnd(dict, idx)
	if end < 0 {
		return ""
	}
	return pdfStringUnescape(dict[idx+1 : end])
}

// attachLiteralEnd scans a PDF literal string opening at data[start] == '('
// and returns the index of its matching ')', honouring backslash escapes and
// nested unescaped parens, or -1 when unterminated.
func attachLiteralEnd(data []byte, start int) int {
	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++ // skip the escaped byte
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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
// by this package and standard writers: \\ \( \) \r \n \t and octal \ddd.
func pdfStringUnescape(s []byte) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch {
			case s[i] == 'n':
				b.WriteByte('\n')
			case s[i] == 'r':
				b.WriteByte('\r')
			case s[i] == 't':
				b.WriteByte('\t')
			case s[i] >= '0' && s[i] <= '7':
				v := 0
				for d := 0; d < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7'; d++ {
					v = v<<3 | int(s[i]-'0')
					i++
				}
				i--
				b.WriteByte(byte(v))
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
// replaced. It strips any pre-existing /Names entry — indirect reference or
// inline dictionary — and appends a fresh one before the closing >>.
func injectNamesRef(catalogBody []byte, namesNo int) []byte {
	s := string(attachStripDictKey(catalogBody, "Names"))
	end := strings.LastIndex(s, ">>")
	if end == -1 {
		return []byte(fmt.Sprintf("<</Names %d 0 R>>", namesNo))
	}
	newRef := fmt.Sprintf("/Names %d 0 R", namesNo)
	return []byte(s[:end] + newRef + s[end:])
}

// attachDictInner returns the content between the outer << >> of a dictionary
// object (as returned by GetObject, which includes the "N G obj"/"endobj"
// wrapper), or body unchanged when it is already inner content (as returned
// by attachFindSubDict for inline dictionaries).
func attachDictInner(body []byte) []byte {
	i := 0
	for i < len(body) && attachIsSpace(body[i]) {
		i++
	}
	if m := regexp.MustCompile(`^\d+\s+\d+\s+obj\b`).Find(body[i:]); m != nil {
		i += len(m)
		for i < len(body) && attachIsSpace(body[i]) {
			i++
		}
	}
	if i+1 < len(body) && body[i] == '<' && body[i+1] == '<' {
		if end := attachMatchDelimited(body, i, '<', '>'); end > i {
			return body[i+2 : end-1]
		}
	}
	return body[i:]
}

// attachStripDictKey returns dict with its first /key entry and the entry's
// value removed. Dictionary, array, string (literal and hex), indirect
// reference, and single-token values are recognised; dict is returned
// unchanged when key is absent or its value cannot be delimited.
func attachStripDictKey(dict []byte, key string) []byte {
	loc := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\b`).FindIndex(dict)
	if loc == nil {
		return dict
	}
	i := loc[1]
	for i < len(dict) && attachIsSpace(dict[i]) {
		i++
	}
	if i >= len(dict) {
		return dict
	}
	end := -1 // index of the value's last byte
	switch {
	case dict[i] == '<' && i+1 < len(dict) && dict[i+1] == '<':
		end = attachMatchDelimited(dict, i, '<', '>')
	case dict[i] == '[':
		end = attachMatchDelimited(dict, i, '[', ']')
	case dict[i] == '(':
		end = attachLiteralEnd(dict, i)
	case dict[i] == '<':
		if e := bytes.IndexByte(dict[i:], '>'); e >= 0 {
			end = i + e
		}
	default:
		if m := regexp.MustCompile(`^(\d+\s+\d+\s+R|/?[^\s/<>\[\]()]+)`).Find(dict[i:]); m != nil {
			end = i + len(m) - 1
		}
	}
	if end < 0 {
		return dict
	}
	out := make([]byte, 0, len(dict)-(end+1-loc[0]))
	out = append(out, dict[:loc[0]]...)
	out = append(out, dict[end+1:]...)
	return out
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
