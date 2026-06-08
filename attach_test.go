package pdfer

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// buildAttachBasePDF returns a minimal single-page PDF to attach files to.
func buildAttachBasePDF(t *testing.T) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	page.Content().
		BeginText().
		SetFont(page.AddStandardFont("Helvetica"), 18).
		SetTextPosition(72, 720).
		ShowText("Host document").
		EndText()
	builder.FinalizePage(page)
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("build base PDF: %v", err)
	}
	return b
}

// streamPayload extracts the raw stream bytes from an object body returned by
// GetObjectContent, using the dictionary /Length.
func streamPayload(t *testing.T, content []byte) []byte {
	t.Helper()
	s := bytes.Index(content, []byte("stream"))
	if s < 0 {
		t.Fatal("no stream keyword in object body")
	}
	s += len("stream")
	if s < len(content) && content[s] == '\r' {
		s++
	}
	if s < len(content) && content[s] == '\n' {
		s++
	}
	if m := regexp.MustCompile(`/Length\s+(\d+)`).FindSubmatch(content); m != nil {
		n, _ := strconv.Atoi(string(m[1]))
		if n > 0 && s+n <= len(content) {
			return content[s : s+n]
		}
	}
	e := bytes.LastIndex(content, []byte("endstream"))
	if e < s {
		t.Fatal("no endstream after stream data")
	}
	return content[s:e]
}

func inflate(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("zlib reader: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("inflate: %v", err)
	}
	return out
}

func TestEmbedAttachments_Unencrypted(t *testing.T) {
	base := buildAttachBasePDF(t)

	payload := []byte("hello attachment payload, unencrypted")
	out, err := EmbedAttachments(base, []FileAttachment{
		{Name: "note.txt", Data: payload, MimeType: "text/plain"},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments: %v", err)
	}

	if !bytes.Contains(out, []byte("/EmbeddedFile")) {
		t.Error("output missing /EmbeddedFile")
	}
	if !bytes.Contains(out, []byte("/EmbeddedFiles")) {
		t.Error("output missing /EmbeddedFiles name tree")
	}
	if !bytes.Contains(out, []byte("(note.txt)")) {
		t.Error("filename should appear in cleartext for an unencrypted PDF")
	}

	// Round-trip: read the embedded stream back and inflate it.
	streamNo := embeddedStreamObjNum(t, base, nil)
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	body, err := pdf.GetObjectContent(streamNo)
	if err != nil {
		t.Fatalf("read embedded stream %d: %v", streamNo, err)
	}
	if got := inflate(t, streamPayload(t, body)); !bytes.Equal(got, payload) {
		t.Errorf("payload round-trip mismatch: got %q want %q", got, payload)
	}
}

func TestEmbedAttachments_Encrypted(t *testing.T) {
	base := buildAttachBasePDF(t)
	password := []byte("s3cret")

	enc, err := EncryptPDF(base, password, nil, false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	// Object number the embedded stream will receive (first free slot).
	streamNo := embeddedStreamObjNum(t, enc, password)
	filespecNo := streamNo + 1

	payload := []byte("TOP SECRET attachment contents that must be encrypted")
	out, err := EmbedAttachmentsWithPassword(enc, []FileAttachment{
		{Name: "secret.txt", Data: payload, MimeType: "text/plain"},
	}, password)
	if err != nil {
		t.Fatalf("EmbedAttachments (encrypted): %v", err)
	}

	// --- The /ID must be carried into the new trailer (the reported bug). ---
	lastTrailer := out[bytes.LastIndex(out, []byte("trailer")):]
	if !bytes.Contains(lastTrailer, []byte("/ID")) {
		t.Error("new trailer dropped /ID for an encrypted PDF")
	}
	if !bytes.Contains(lastTrailer, []byte("/Encrypt")) {
		t.Error("new trailer dropped /Encrypt")
	}

	// --- The stream must actually be encrypted: cleartext must not leak. ---
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	zw.Write(payload)
	zw.Close()
	if bytes.Contains(out, compressed.Bytes()) {
		t.Error("compressed attachment bytes appear in cleartext; stream was not encrypted")
	}
	// pdfer encrypts with /StrF /Identity, so strings (the filename) are stored
	// in the clear by design; encrypting them would be unreadable to conformant
	// viewers. The stream (/StmF /StdCF) is still encrypted, checked above.
	if !bytes.Contains(out, []byte("(secret.txt)")) {
		t.Error("filename should be cleartext for a /StrF /Identity document")
	}

	// --- Round-trip with the password: stream decrypts + inflates to payload. ---
	pdf, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("reopen encrypted: %v", err)
	}
	if pdf.Encryption() == nil {
		t.Fatal("reopened document is not encrypted")
	}
	body, err := pdf.GetObjectContent(streamNo)
	if err != nil {
		t.Fatalf("read embedded stream %d: %v", streamNo, err)
	}
	if got := inflate(t, streamPayload(t, body)); !bytes.Equal(got, payload) {
		t.Errorf("encrypted payload round-trip mismatch: got %q want %q", got, payload)
	}

	// Filespec filename decrypts back to the original name.
	fsBody, err := pdf.GetObjectContent(filespecNo)
	if err != nil {
		t.Fatalf("read filespec %d: %v", filespecNo, err)
	}
	if !bytes.Contains(fsBody, []byte("secret.txt")) {
		t.Errorf("filespec did not decrypt to filename; got %q", fsBody)
	}
}

// embeddedStreamObjNum returns the object number the first appended attachment
// stream receives: the document's next free object number (trailer /Size).
func embeddedStreamObjNum(t *testing.T, pdfBytes, password []byte) int {
	t.Helper()
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("parse for size: %v", err)
	}
	tr := pdf.Trailer()
	if tr == nil || tr.Size <= 0 {
		t.Fatalf("could not determine trailer size")
	}
	return tr.Size
}
