// Package incremental writes PDF incremental updates: new and replacement
// objects appended after the original bytes, followed by a fresh xref section
// and trailer that chains to the previous one via /Prev.
//
// It centralises the trailer/xref/encryption invariants that every incremental
// writer in this module must uphold — most importantly carrying the document
// /ID forward and encrypting appended strings and streams when the source PDF
// is encrypted. Hand-rolling these per call site previously caused encrypted
// attachments to be written in the clear with a dropped /ID.
package incremental

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// Update accumulates an incremental update against an original PDF.
//
// Object numbers are reserved with Reserve, bodies are appended with AddObject
// (dictionaries) or AddStream (stream objects), and the finished PDF is produced
// by Bytes. When the source document is encrypted (enc != nil) all appended
// strings and stream data are encrypted with the document's per-object keys.
type Update struct {
	rootRef    string
	infoRef    string
	encryptRef string
	enc        *types.PDFEncryption
	prevXRef   int64
	idArray    []byte // raw "[<...><...>]" carried from the source trailer
	xrefStream bool   // source's active xref is a cross-reference stream

	nextObj int
	buf     bytes.Buffer
	offsets map[int]int64
}

var (
	lengthPat    = regexp.MustCompile(`/Length\s+\d+`)
	idPat        = regexp.MustCompile(`/ID\s*(\[[^\]]*\])`)
	startXRefPat = regexp.MustCompile(`startxref\s+(\d+)`)
)

// New starts an incremental update against original using the parsed trailer.
// If enc is non-nil, appended content is encrypted for the document.
func New(original []byte, trailer *parse.TrailerInfo, enc *types.PDFEncryption) (*Update, error) {
	if trailer == nil {
		return nil, fmt.Errorf("incremental: nil trailer")
	}
	prev := findStartXRef(original)
	if prev < 0 {
		return nil, fmt.Errorf("incremental: could not locate startxref")
	}
	u := &Update{
		rootRef:    trailer.RootRef,
		infoRef:    trailer.InfoRef,
		encryptRef: trailer.EncryptRef,
		enc:        enc,
		prevXRef:   prev,
		idArray:    extractTrailerID(original, prev),
		xrefStream: xrefIsStream(original, prev),
		nextObj:    trailer.Size,
		offsets:    make(map[int]int64),
	}
	u.buf.Write(original)
	u.buf.WriteByte('\n')
	return u, nil
}

// Reserve allocates and returns the next free object number. Callers reserve all
// numbers they need before adding bodies so forward references resolve.
func (u *Update) Reserve() int {
	n := u.nextObj
	u.nextObj++
	return n
}

// Encrypted reports whether appended content will be encrypted.
func (u *Update) Encrypted() bool { return u.enc != nil }

// AddObject appends a dictionary (non-stream) object. body must be the full
// object body including the surrounding << >>. Any string values it contains are
// encrypted when the document is encrypted.
func (u *Update) AddObject(num, gen int, body []byte) error {
	if u.enc != nil {
		enc, err := encrypt.EncryptStringsInContent(body, num, gen, u.enc)
		if err != nil {
			return fmt.Errorf("incremental: encrypt obj %d: %w", num, err)
		}
		body = enc
	}
	u.offsets[num] = int64(u.buf.Len())
	fmt.Fprintf(&u.buf, "%d %d obj\n", num, gen)
	u.buf.Write(body)
	u.buf.WriteString("\nendobj\n")
	return nil
}

// AddStream appends a stream object. dict is the stream dictionary including the
// surrounding << >> but WITHOUT a /Length entry (it is computed and injected
// here). When the document is encrypted, data and any strings in dict are
// encrypted, and /Length reflects the encrypted byte count.
func (u *Update) AddStream(num, gen int, dict, data []byte) error {
	if u.enc != nil {
		var err error
		if data, err = encrypt.EncryptObject(data, num, gen, u.enc); err != nil {
			return fmt.Errorf("incremental: encrypt stream %d: %w", num, err)
		}
		if dict, err = encrypt.EncryptStringsInContent(dict, num, gen, u.enc); err != nil {
			return fmt.Errorf("incremental: encrypt stream dict %d: %w", num, err)
		}
	}
	dict = setLength(dict, len(data))

	u.offsets[num] = int64(u.buf.Len())
	fmt.Fprintf(&u.buf, "%d %d obj\n", num, gen)
	u.buf.Write(dict)
	u.buf.WriteString("\nstream\n")
	u.buf.Write(data)
	u.buf.WriteString("\nendstream\nendobj\n")
	return nil
}

// Bytes finalises the update: xref section, trailer (preserving Root/Info/
// Encrypt/ID, chaining via /Prev) and startxref.
//
// The cross-reference form mirrors the source: a classical xref table + trailer
// when the previous section is a table, a cross-reference stream when the
// previous section is a stream (ISO 32000-1 §7.5.8.4 requires updates to a
// pure xref-stream file to also use xref streams).
func (u *Update) Bytes() ([]byte, error) {
	if len(u.offsets) == 0 {
		return nil, fmt.Errorf("incremental: no objects added")
	}
	if u.xrefStream {
		return u.bytesXRefStream()
	}

	xrefStart := int64(u.buf.Len())
	u.buf.WriteString("xref\n")

	nums := make([]int, 0, len(u.offsets))
	for n := range u.offsets {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	writeXRefSubsections(&u.buf, u.offsets, nums)

	u.buf.WriteString("trailer\n<<")
	fmt.Fprintf(&u.buf, "/Size %d", u.nextObj)
	fmt.Fprintf(&u.buf, "/Root %s", u.rootRef)
	if u.infoRef != "" {
		fmt.Fprintf(&u.buf, "/Info %s", u.infoRef)
	}
	if u.encryptRef != "" {
		fmt.Fprintf(&u.buf, "/Encrypt %s", u.encryptRef)
	}
	// Carry the document /ID forward. The standard security handler derives the
	// encryption key from /ID[0], so dropping it breaks decryption of every
	// appended object in encrypted files; for plain files it preserves identity
	// across the update.
	if len(u.idArray) > 0 {
		u.buf.WriteString("/ID ")
		u.buf.Write(u.idArray)
	}
	fmt.Fprintf(&u.buf, "/Prev %d", u.prevXRef)
	u.buf.WriteString(">>\n")
	fmt.Fprintf(&u.buf, "startxref\n%d\n%%%%EOF\n", xrefStart)

	return u.buf.Bytes(), nil
}

// bytesXRefStream finalises the update with a cross-reference stream instead of
// a classical table. Trailer entries (Size/Root/Info/Encrypt/ID/Prev) live in
// the stream dictionary. The stream itself is never encrypted (ISO 32000-1
// §7.5.8.2), so it is written directly rather than through AddStream.
func (u *Update) bytesXRefStream() ([]byte, error) {
	// The xref stream is itself an object and must appear in its own table.
	xrefNum := u.Reserve()
	xrefStart := int64(u.buf.Len())
	u.offsets[xrefNum] = xrefStart

	nums := make([]int, 0, len(u.offsets))
	for n := range u.offsets {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Field widths: type (1), byte offset (enough for the largest offset,
	// which is always the xref stream's own), generation (2).
	const w1, w3 = 1, 2
	w2 := bytesNeeded(xrefStart)

	var index, data bytes.Buffer
	i := 0
	for i < len(nums) {
		j := i + 1
		for j < len(nums) && nums[j] == nums[j-1]+1 {
			j++
		}
		fmt.Fprintf(&index, " %d %d", nums[i], j-i)
		for k := i; k < j; k++ {
			entry := make([]byte, w1+w2+w3)
			entry[0] = 1 // type 1: in-use, at byte offset, generation 0
			putBigEndian(entry[w1:w1+w2], u.offsets[nums[k]])
			data.Write(entry)
		}
		i = j
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(data.Bytes()); err != nil {
		return nil, fmt.Errorf("incremental: compress xref stream: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("incremental: compress xref stream: %w", err)
	}

	fmt.Fprintf(&u.buf, "%d 0 obj\n<</Type/XRef", xrefNum)
	fmt.Fprintf(&u.buf, "/Size %d", u.nextObj)
	fmt.Fprintf(&u.buf, "/W[%d %d %d]", w1, w2, w3)
	fmt.Fprintf(&u.buf, "/Index[%s]", bytes.TrimSpace(index.Bytes()))
	fmt.Fprintf(&u.buf, "/Root %s", u.rootRef)
	if u.infoRef != "" {
		fmt.Fprintf(&u.buf, "/Info %s", u.infoRef)
	}
	if u.encryptRef != "" {
		fmt.Fprintf(&u.buf, "/Encrypt %s", u.encryptRef)
	}
	if len(u.idArray) > 0 {
		u.buf.WriteString("/ID ")
		u.buf.Write(u.idArray)
	}
	fmt.Fprintf(&u.buf, "/Prev %d", u.prevXRef)
	fmt.Fprintf(&u.buf, "/Filter/FlateDecode/Length %d>>\nstream\n", compressed.Len())
	u.buf.Write(compressed.Bytes())
	u.buf.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&u.buf, "startxref\n%d\n%%%%EOF\n", xrefStart)

	return u.buf.Bytes(), nil
}

// UsesXRefStream reports whether the PDF's active cross-reference section is a
// cross-reference stream (PDF 1.5+) rather than a classical xref table. Returns
// false when no startxref can be located.
func UsesXRefStream(pdfBytes []byte) bool {
	prev := findStartXRef(pdfBytes)
	return prev >= 0 && xrefIsStream(pdfBytes, prev)
}

// xrefIsStream reports whether the cross-reference section at offset is a
// cross-reference stream (an indirect object) rather than a classical table
// (the keyword "xref").
func xrefIsStream(pdfBytes []byte, offset int64) bool {
	if offset < 0 || offset >= int64(len(pdfBytes)) {
		return false
	}
	tail := pdfBytes[offset:]
	if len(tail) > 32 {
		tail = tail[:32]
	}
	return !bytes.HasPrefix(bytes.TrimLeft(tail, " \t\r\n"), []byte("xref"))
}

// bytesNeeded returns how many bytes are required to represent n big-endian.
func bytesNeeded(n int64) int {
	w := 1
	for n > 0xff {
		w++
		n >>= 8
	}
	return w
}

// putBigEndian writes n into dst big-endian, using all of dst.
func putBigEndian(dst []byte, n int64) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = byte(n)
		n >>= 8
	}
}

// setLength removes any existing /Length entry from a stream dictionary and
// inserts /Length n immediately after the opening <<.
func setLength(dict []byte, n int) []byte {
	dict = lengthPat.ReplaceAll(dict, nil)
	open := bytes.Index(dict, []byte("<<"))
	if open < 0 {
		return []byte(fmt.Sprintf("<</Length %d>>%s", n, dict))
	}
	at := open + 2
	out := make([]byte, 0, len(dict)+16)
	out = append(out, dict[:at]...)
	out = append(out, []byte(fmt.Sprintf("/Length %d", n))...)
	out = append(out, dict[at:]...)
	return out
}

// extractTrailerID returns the raw "[<...><...>]" /ID array from the active
// cross-reference section, or nil if absent. The search is scoped to the bytes
// from xrefOffset (the classic trailer dict or the xref-stream object, both of
// which hold /ID as plaintext) to EOF, so an /ID-like token inside an earlier
// body object cannot be mistaken for the trailer identifier. Falls back to the
// last match in the whole file if the scoped search finds nothing.
func extractTrailerID(pdfBytes []byte, xrefOffset int64) []byte {
	if xrefOffset >= 0 && xrefOffset < int64(len(pdfBytes)) {
		if m := idPat.FindAllSubmatch(pdfBytes[xrefOffset:], -1); len(m) > 0 {
			return m[len(m)-1][1]
		}
	}
	m := idPat.FindAllSubmatch(pdfBytes, -1)
	if len(m) == 0 {
		return nil
	}
	return m[len(m)-1][1]
}

// findStartXRef returns the offset from the last startxref in the file tail.
func findStartXRef(pdfBytes []byte) int64 {
	searchLen := 2048
	if searchLen > len(pdfBytes) {
		searchLen = len(pdfBytes)
	}
	tail := pdfBytes[len(pdfBytes)-searchLen:]
	matches := startXRefPat.FindAllSubmatch(tail, -1)
	if len(matches) == 0 {
		return -1
	}
	v, err := strconv.ParseInt(string(matches[len(matches)-1][1]), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

// writeXRefSubsections writes xref entries grouped into contiguous subsections.
func writeXRefSubsections(buf *bytes.Buffer, offsets map[int]int64, objNums []int) {
	i := 0
	for i < len(objNums) {
		j := i + 1
		for j < len(objNums) && objNums[j] == objNums[j-1]+1 {
			j++
		}
		fmt.Fprintf(buf, "%d %d\n", objNums[i], j-i)
		for k := i; k < j; k++ {
			fmt.Fprintf(buf, "%010d %05d n \n", offsets[objNums[k]], 0)
		}
		i = j
	}
}
