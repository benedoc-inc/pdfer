package pdfer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/incremental"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
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
	// pdfer encrypts with /StrF /StdCF, so strings (the filename) must be
	// ciphertext in the raw bytes, matching the declared filter.
	if bytes.Contains(out, []byte("secret.txt")) {
		t.Error("filename appears in cleartext for a /StrF /StdCF document")
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

func TestListAttachments_RoundTrip(t *testing.T) {
	base := buildAttachBasePDF(t)

	in := []FileAttachment{
		{Name: "report.txt", Data: []byte("hello world\nsecond line"), MimeType: "text/plain"},
		{Name: "data.bin", Data: bytes.Repeat([]byte{0x00, 0x01, 0x02, 0xfe, 0xff}, 100), MimeType: "application/octet-stream"},
		{Name: "weird (name).pdf", Data: []byte("%PDF-1.4 fake"), MimeType: "application/pdf"},
	}

	withAttachments, err := EmbedAttachments(base, in)
	if err != nil {
		t.Fatalf("EmbedAttachments: %v", err)
	}

	got, err := ListAttachments(withAttachments)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d attachments, want %d", len(got), len(in))
	}

	byName := make(map[string]FileAttachment, len(got))
	for _, a := range got {
		byName[a.Name] = a
	}
	for _, want := range in {
		a, ok := byName[want.Name]
		if !ok {
			t.Errorf("attachment %q missing from result", want.Name)
			continue
		}
		if !bytes.Equal(a.Data, want.Data) {
			t.Errorf("attachment %q data mismatch: got %d bytes, want %d", want.Name, len(a.Data), len(want.Data))
		}
		if a.MimeType != want.MimeType {
			t.Errorf("attachment %q mime: got %q, want %q", want.Name, a.MimeType, want.MimeType)
		}
	}
}

// TestListAttachments_Encrypted confirms the reader decrypts embedded-file
// streams written by EmbedAttachmentsWithPassword.
func TestListAttachments_Encrypted(t *testing.T) {
	base := buildAttachBasePDF(t)
	password := []byte("s3cret")
	enc, err := EncryptPDF(base, password, nil, false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	payload := []byte("TOP SECRET attachment contents")
	out, err := EmbedAttachmentsWithPassword(enc, []FileAttachment{
		{Name: "secret.txt", Data: payload, MimeType: "text/plain"},
	}, password)
	if err != nil {
		t.Fatalf("EmbedAttachmentsWithPassword: %v", err)
	}

	got, err := ListAttachmentsWithPassword(out, password)
	if err != nil {
		t.Fatalf("ListAttachmentsWithPassword: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d attachments, want 1", len(got))
	}
	if got[0].Name != "secret.txt" || !bytes.Equal(got[0].Data, payload) {
		t.Fatalf("unexpected attachment: name=%q data=%q", got[0].Name, got[0].Data)
	}
}

// TestListAttachments_EncryptedDefaultMime pins the parse-layer fix for
// stream-keyword location: the default MIME type yields a /Subtype name
// (/application#2Foctet-stream) containing the bytes "stream", which made the
// decryptor slice the wrong range and return the stream still encrypted.
func TestListAttachments_EncryptedDefaultMime(t *testing.T) {
	base := buildAttachBasePDF(t)
	password := []byte("s3cret")
	enc, err := EncryptPDF(base, password, nil, false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	payload := []byte("binary payload, default mime")
	out, err := EmbedAttachmentsWithPassword(enc, []FileAttachment{
		{Name: "blob.bin", Data: payload},
	}, password)
	if err != nil {
		t.Fatalf("EmbedAttachmentsWithPassword: %v", err)
	}

	got, err := ListAttachmentsWithPassword(out, password)
	if err != nil {
		t.Fatalf("ListAttachmentsWithPassword: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d attachments, want 1", len(got))
	}
	if got[0].Name != "blob.bin" || !bytes.Equal(got[0].Data, payload) {
		t.Fatalf("unexpected attachment: name=%q data=%q", got[0].Name, got[0].Data)
	}
	if got[0].MimeType != "application/octet-stream" {
		t.Errorf("mime: got %q, want application/octet-stream", got[0].MimeType)
	}
}

func TestListAttachments_None(t *testing.T) {
	got, err := ListAttachments(buildAttachBasePDF(t))
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no attachments, got %d", len(got))
	}
}

// TestListAttachments_NestedNameTree verifies the reader descends through
// intermediate /Kids nodes, which EmbedAttachments never produces (it writes a
// flat tree) but other PDF producers do.
func TestListAttachments_NestedNameTree(t *testing.T) {
	pdf := buildNestedNameTreePDF(t, "nested.txt", []byte("nested content"))
	got, err := ListAttachments(pdf)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d attachments, want 1", len(got))
	}
	if got[0].Name != "nested.txt" || !bytes.Equal(got[0].Data, []byte("nested content")) {
		t.Fatalf("unexpected attachment: name=%q data=%q", got[0].Name, got[0].Data)
	}
}

// TestEmbedAttachments_AppendsAcrossBatches is the regression test for the
// reported data loss: a second EmbedAttachments call must preserve the
// attachments embedded by the first, and the merged name tree must be
// lexically sorted regardless of embed order.
func TestEmbedAttachments_AppendsAcrossBatches(t *testing.T) {
	base := buildAttachBasePDF(t)

	out, err := EmbedAttachments(base, []FileAttachment{
		{Name: "c.txt", Data: []byte("C"), MimeType: "text/plain"},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments batch 1: %v", err)
	}
	out, err = EmbedAttachments(out, []FileAttachment{
		{Name: "a.txt", Data: []byte("A"), MimeType: "text/plain"},
		{Name: "b.txt", Data: []byte("B"), MimeType: "text/plain"},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments batch 2: %v", err)
	}

	got, err := ListAttachments(out)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	want := map[string]string{"a.txt": "A", "b.txt": "B", "c.txt": "C"}
	if len(got) != len(want) {
		t.Fatalf("got %d attachments %v, want %d", len(got), attachmentNames(got), len(want))
	}
	for i, a := range got {
		if data, ok := want[a.Name]; !ok || string(a.Data) != data {
			t.Errorf("attachment %q data %q, want %q", a.Name, a.Data, data)
		}
		if i > 0 && got[i-1].Name > a.Name {
			t.Errorf("name tree not sorted: %q before %q", got[i-1].Name, a.Name)
		}
	}
}

// TestEmbedAttachments_NameCollisionRenames verifies the deterministic -1, -2
// rename when a new attachment collides with an existing entry or with another
// file in the same batch.
func TestEmbedAttachments_NameCollisionRenames(t *testing.T) {
	base := buildAttachBasePDF(t)

	out, err := EmbedAttachments(base, []FileAttachment{
		{Name: "report.txt", Data: []byte("first")},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments batch 1: %v", err)
	}
	out, err = EmbedAttachments(out, []FileAttachment{
		{Name: "report.txt", Data: []byte("second")},
		{Name: "report.txt", Data: []byte("third")},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments batch 2: %v", err)
	}

	got, err := ListAttachments(out)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	want := map[string]string{"report.txt": "first", "report-1.txt": "second", "report-2.txt": "third"}
	if len(got) != len(want) {
		t.Fatalf("got %d attachments %v, want %d", len(got), attachmentNames(got), len(want))
	}
	for _, a := range got {
		if data, ok := want[a.Name]; !ok || string(a.Data) != data {
			t.Errorf("attachment %q data %q, want %q", a.Name, a.Data, data)
		}
	}
}

// TestEmbedAttachments_EncryptedAcrossBatches confirms the merge reads the
// existing (encrypted) name tree and re-encrypts the rewritten objects.
func TestEmbedAttachments_EncryptedAcrossBatches(t *testing.T) {
	base := buildAttachBasePDF(t)
	password := []byte("s3cret")
	enc, err := EncryptPDF(base, password, nil, false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	out, err := EmbedAttachmentsWithPassword(enc, []FileAttachment{
		{Name: "first.txt", Data: []byte("payload one")},
	}, password)
	if err != nil {
		t.Fatalf("EmbedAttachmentsWithPassword batch 1: %v", err)
	}
	out, err = EmbedAttachmentsWithPassword(out, []FileAttachment{
		{Name: "second.txt", Data: []byte("payload two")},
	}, password)
	if err != nil {
		t.Fatalf("EmbedAttachmentsWithPassword batch 2: %v", err)
	}

	if bytes.Contains(out, []byte("first.txt")) || bytes.Contains(out, []byte("second.txt")) {
		t.Error("filename appears in cleartext for a /StrF /StdCF document")
	}

	got, err := ListAttachmentsWithPassword(out, password)
	if err != nil {
		t.Fatalf("ListAttachmentsWithPassword: %v", err)
	}
	want := map[string]string{"first.txt": "payload one", "second.txt": "payload two"}
	if len(got) != len(want) {
		t.Fatalf("got %d attachments %v, want %d", len(got), attachmentNames(got), len(want))
	}
	for _, a := range got {
		if data, ok := want[a.Name]; !ok || string(a.Data) != data {
			t.Errorf("attachment %q data %q, want %q", a.Name, a.Data, data)
		}
	}
}

// TestEmbedAttachments_PreservesSiblingNameTrees verifies that other name
// trees under the catalog's /Names dictionary (here /Dests) survive an embed.
func TestEmbedAttachments_PreservesSiblingNameTrees(t *testing.T) {
	pdf := buildSiblingNamesPDF(t, "seed.txt", []byte("seed content"))

	out, err := EmbedAttachments(pdf, []FileAttachment{
		{Name: "new.txt", Data: []byte("new content")},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments: %v", err)
	}

	got, err := ListAttachments(out)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d attachments %v, want 2", len(got), attachmentNames(got))
	}

	p, err := parse.Open(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	trailer := p.Trailer()
	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		t.Fatalf("bad root ref: %v", err)
	}
	catalog, err := p.GetObject(rootNum)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	namesDict, err := attachResolveSubDict(p, catalog, "Names")
	if err != nil || namesDict == nil {
		t.Fatalf("catalog /Names unreadable: %v", err)
	}
	dests := attachFindSubDict(namesDict, "Dests")
	if dests == nil {
		t.Fatal("/Dests sibling name tree was dropped by EmbedAttachments")
	}
	if !bytes.Contains(dests, []byte("(intro)")) {
		t.Errorf("/Dests content altered: %q", dests)
	}
}

// TestEmbedAttachments_InlineNamesDict covers a catalog whose /Names entry is
// written inline (<<...>>) rather than as an indirect reference: the embed
// must merge with it and must not leave a duplicate /Names key behind.
func TestEmbedAttachments_InlineNamesDict(t *testing.T) {
	pdf := buildInlineNamesPDF(t, "seed.txt", []byte("seed content"))

	out, err := EmbedAttachments(pdf, []FileAttachment{
		{Name: "new.txt", Data: []byte("new content")},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments: %v", err)
	}

	got, err := ListAttachments(out)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	want := map[string]string{"seed.txt": "seed content", "new.txt": "new content"}
	if len(got) != len(want) {
		t.Fatalf("got %d attachments %v, want %d", len(got), attachmentNames(got), len(want))
	}
	for _, a := range got {
		if data, ok := want[a.Name]; !ok || string(a.Data) != data {
			t.Errorf("attachment %q data %q, want %q", a.Name, a.Data, data)
		}
	}

	// The rewritten catalog must hold exactly one /Names key (the new ref).
	p, err := parse.Open(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	rootNum, err := parseRefNum(p.Trailer().RootRef)
	if err != nil {
		t.Fatalf("bad root ref: %v", err)
	}
	catalog, err := p.GetObjectContent(rootNum)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if n := len(regexp.MustCompile(`/Names\b`).FindAll(catalog, -1)); n != 1 {
		t.Errorf("catalog has %d /Names keys, want 1: %q", n, catalog)
	}
}

// TestEmbedAttachments_MergesNestedNameTree embeds onto a document whose
// existing tree uses intermediate /Kids nodes: the merge must flatten it
// without losing the nested entry.
func TestEmbedAttachments_MergesNestedNameTree(t *testing.T) {
	pdf := buildNestedNameTreePDF(t, "nested.txt", []byte("nested content"))

	out, err := EmbedAttachments(pdf, []FileAttachment{
		{Name: "extra.txt", Data: []byte("extra content")},
	})
	if err != nil {
		t.Fatalf("EmbedAttachments: %v", err)
	}

	got, err := ListAttachments(out)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	want := map[string]string{"nested.txt": "nested content", "extra.txt": "extra content"}
	if len(got) != len(want) {
		t.Fatalf("got %d attachments %v, want %d", len(got), attachmentNames(got), len(want))
	}
	for _, a := range got {
		if data, ok := want[a.Name]; !ok || string(a.Data) != data {
			t.Errorf("attachment %q data %q, want %q", a.Name, a.Data, data)
		}
	}
}

func attachmentNames(atts []FileAttachment) []string {
	names := make([]string, len(atts))
	for i, a := range atts {
		names[i] = a.Name
	}
	return names
}

// buildSiblingNamesPDF embeds one file whose /Names dictionary also carries a
// /Dests name tree, exercising sibling preservation during merges.
func buildSiblingNamesPDF(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	base := buildAttachBasePDF(t)

	pdf, err := parse.OpenWithOptions(base, parse.ParseOptions{})
	if err != nil {
		t.Fatalf("parse base PDF: %v", err)
	}
	trailer := pdf.Trailer()
	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		t.Fatalf("bad root ref: %v", err)
	}
	catalog, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	upd, err := incremental.New(base, trailer, pdf.Encryption())
	if err != nil {
		t.Fatalf("incremental.New: %v", err)
	}

	streamNo := upd.Reserve()
	if err := upd.AddStream(streamNo, 0, []byte("<</Type /EmbeddedFile /Filter /FlateDecode>>"), compressBytes(data)); err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	esc := pdfStringEscape(name)
	filespecNo := upd.Reserve()
	if err := upd.AddObject(filespecNo, 0, []byte(fmt.Sprintf("<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>", esc, esc, streamNo))); err != nil {
		t.Fatalf("AddObject filespec: %v", err)
	}

	namesNo := upd.Reserve()
	names := fmt.Sprintf("<</Dests <</Names [(intro) 1 0 R]>> /EmbeddedFiles <</Names [(%s) %d 0 R]>>>>", esc, filespecNo)
	if err := upd.AddObject(namesNo, 0, []byte(names)); err != nil {
		t.Fatalf("AddObject names: %v", err)
	}

	if err := upd.AddObject(rootNum, 0, injectNamesRef(catalog, namesNo)); err != nil {
		t.Fatalf("AddObject catalog: %v", err)
	}

	out, err := upd.Bytes()
	if err != nil {
		t.Fatalf("incremental.Bytes: %v", err)
	}
	return out
}

// buildInlineNamesPDF embeds one file with the /Names dictionary written
// inline in the catalog instead of as an indirect reference.
func buildInlineNamesPDF(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	base := buildAttachBasePDF(t)

	pdf, err := parse.OpenWithOptions(base, parse.ParseOptions{})
	if err != nil {
		t.Fatalf("parse base PDF: %v", err)
	}
	trailer := pdf.Trailer()
	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		t.Fatalf("bad root ref: %v", err)
	}
	catalog, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	upd, err := incremental.New(base, trailer, pdf.Encryption())
	if err != nil {
		t.Fatalf("incremental.New: %v", err)
	}

	streamNo := upd.Reserve()
	if err := upd.AddStream(streamNo, 0, []byte("<</Type /EmbeddedFile /Filter /FlateDecode>>"), compressBytes(data)); err != nil {
		t.Fatalf("AddStream: %v", err)
	}
	esc := pdfStringEscape(name)
	filespecNo := upd.Reserve()
	if err := upd.AddObject(filespecNo, 0, []byte(fmt.Sprintf("<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>", esc, esc, streamNo))); err != nil {
		t.Fatalf("AddObject filespec: %v", err)
	}

	inline := fmt.Sprintf("/Names <</EmbeddedFiles <</Names [(%s) %d 0 R]>>>>", esc, filespecNo)
	end := bytes.LastIndex(catalog, []byte(">>"))
	if end == -1 {
		t.Fatal("catalog has no closing >>")
	}
	newCatalog := append(append(append([]byte{}, catalog[:end]...), inline...), catalog[end:]...)
	if err := upd.AddObject(rootNum, 0, newCatalog); err != nil {
		t.Fatalf("AddObject catalog: %v", err)
	}

	out, err := upd.Bytes()
	if err != nil {
		t.Fatalf("incremental.Bytes: %v", err)
	}
	return out
}

// buildNestedNameTreePDF embeds a single file but routes it through an
// intermediate /Kids node, producing the nested name-tree layout that
// EmbedAttachments (which writes flat trees) never emits. It drives the shared
// core/incremental writer directly.
func buildNestedNameTreePDF(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	base := buildAttachBasePDF(t)

	pdf, err := parse.OpenWithOptions(base, parse.ParseOptions{})
	if err != nil {
		t.Fatalf("parse base PDF: %v", err)
	}
	trailer := pdf.Trailer()
	rootNum, err := parseRefNum(trailer.RootRef)
	if err != nil {
		t.Fatalf("bad root ref: %v", err)
	}
	catalog, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	upd, err := incremental.New(base, trailer, pdf.Encryption())
	if err != nil {
		t.Fatalf("incremental.New: %v", err)
	}

	streamNo := upd.Reserve()
	if err := upd.AddStream(streamNo, 0, []byte("<</Type /EmbeddedFile /Filter /FlateDecode>>"), compressBytes(data)); err != nil {
		t.Fatalf("AddStream: %v", err)
	}

	esc := pdfStringEscape(name)
	filespecNo := upd.Reserve()
	if err := upd.AddObject(filespecNo, 0, []byte(fmt.Sprintf("<</Type /Filespec /F (%s) /UF (%s) /EF <</F %d 0 R>>>>", esc, esc, streamNo))); err != nil {
		t.Fatalf("AddObject filespec: %v", err)
	}

	leafNo := upd.Reserve()
	if err := upd.AddObject(leafNo, 0, []byte(fmt.Sprintf("<</Names [(%s) %d 0 R]>>", esc, filespecNo))); err != nil {
		t.Fatalf("AddObject leaf: %v", err)
	}

	rootNode := upd.Reserve()
	if err := upd.AddObject(rootNode, 0, []byte(fmt.Sprintf("<</Kids [%d 0 R]>>", leafNo))); err != nil {
		t.Fatalf("AddObject root node: %v", err)
	}

	namesNo := upd.Reserve()
	if err := upd.AddObject(namesNo, 0, []byte(fmt.Sprintf("<</EmbeddedFiles %d 0 R>>", rootNode))); err != nil {
		t.Fatalf("AddObject names: %v", err)
	}

	if err := upd.AddObject(rootNum, 0, injectNamesRef(catalog, namesNo)); err != nil {
		t.Fatalf("AddObject catalog: %v", err)
	}

	out, err := upd.Bytes()
	if err != nil {
		t.Fatalf("incremental.Bytes: %v", err)
	}
	return out
}
