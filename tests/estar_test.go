package tests

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"testing"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	pdftypes "github.com/benedoc-inc/pdfer/v2/types"
)

// The minimum expected question counts for each eSTAR variant.
const (
	minNonIVDQuestions  = 800
	minIVDQuestions     = 900
	minPreSTARQuestions = 600
)

// TestESTAR_NonIVD_Schema verifies that the Non-IVD eSTAR form parses correctly
// and produces a substantial schema.
func TestESTAR_NonIVD_Schema(t *testing.T) {
	pdfBytes := readESTARPDF(t, "nonivd_estar.pdf")

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm(nonivd_estar.pdf): %v", err)
	}

	schema := form.Schema()
	if schema == nil {
		t.Fatal("Schema() returned nil")
	}
	if len(schema.Questions) < minNonIVDQuestions {
		t.Errorf("expected at least %d questions, got %d", minNonIVDQuestions, len(schema.Questions))
	}
	t.Logf("NonIVD eSTAR: %d questions, %d sections, %d scripts",
		len(schema.Questions), len(schema.Sections), len(schema.Scripts))

	assertSchemaQuality(t, schema, "NonIVD eSTAR")
}

// TestESTAR_IVD_Schema exercises the PDF literal-string unescape fix.
// The IVD eSTAR PDF has escape sequences (\n, \(, \\) in its /U encryption
// field; without proper unescaping, DecryptPDF fails to verify the empty user
// password and returns NO_FORMS instead of the full schema.
func TestESTAR_IVD_Schema(t *testing.T) {
	pdfBytes := readESTARPDF(t, "ivd_estar.pdf")

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm(ivd_estar.pdf): %v", err)
	}

	schema := form.Schema()
	if schema == nil {
		t.Fatal("Schema() returned nil")
	}
	if len(schema.Questions) < minIVDQuestions {
		t.Errorf("expected at least %d questions, got %d", minIVDQuestions, len(schema.Questions))
	}
	t.Logf("IVD eSTAR: %d questions, %d sections, %d scripts",
		len(schema.Questions), len(schema.Sections), len(schema.Scripts))

	assertSchemaQuality(t, schema, "IVD eSTAR")
}

// TestESTAR_PreSTAR_Schema verifies that the PreSTAR form parses correctly.
func TestESTAR_PreSTAR_Schema(t *testing.T) {
	pdfBytes := readESTARPDF(t, "prestar.pdf")

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm(prestar.pdf): %v", err)
	}

	schema := form.Schema()
	if schema == nil {
		t.Fatal("Schema() returned nil")
	}
	if len(schema.Questions) < minPreSTARQuestions {
		t.Errorf("expected at least %d questions, got %d", minPreSTARQuestions, len(schema.Questions))
	}
	t.Logf("PreSTAR: %d questions, %d sections, %d scripts",
		len(schema.Questions), len(schema.Sections), len(schema.Scripts))

	assertSchemaQuality(t, schema, "PreSTAR")
}

// TestESTAR_NonIVD_Fill verifies that the Non-IVD eSTAR can be filled.
func TestESTAR_NonIVD_Fill(t *testing.T) {
	pdfBytes := readESTARPDF(t, "nonivd_estar.pdf")
	assertFillWorks(t, pdfBytes, "nonivd_estar.pdf")
}

// TestESTAR_IVD_Fill verifies that the IVD eSTAR can be filled end-to-end.
func TestESTAR_IVD_Fill(t *testing.T) {
	pdfBytes := readESTARPDF(t, "ivd_estar.pdf")
	assertFillWorks(t, pdfBytes, "ivd_estar.pdf")
}

// TestESTAR_PreSTAR_Fill verifies that the PreSTAR form can be filled.
func TestESTAR_PreSTAR_Fill(t *testing.T) {
	pdfBytes := readESTARPDF(t, "prestar.pdf")
	assertFillWorks(t, pdfBytes, "prestar.pdf")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// assertSchemaQuality checks that the parsed schema meets the criteria for a
// user-friendly form-filling experience:
//
//   - At least 10% of questions have non-empty labels (not just IDs)
//   - No section has a suspiciously long label (>300 chars → likely instruction text leaking)
//   - At least one section has children (tree structure preserved)
//   - Interactive sections use bookmark/caption labels (not raw field names or IMDRF refs)
func assertSchemaQuality(t *testing.T, schema *pdftypes.FormSchema, tag string) {
	t.Helper()

	// 1. Meaningful labels: all questions should have non-empty labels, and
	// ≤2% may fall back to their machine name (label == id).
	labelledCount := 0
	machineNameCount := 0
	for _, q := range schema.Questions {
		if q.Label != "" {
			labelledCount++
		}
		if q.Label == q.ID {
			machineNameCount++
		}
	}
	labelRate := float64(labelledCount) / float64(len(schema.Questions))
	machineNameRate := float64(machineNameCount) / float64(len(schema.Questions))
	if labelRate < 0.10 {
		t.Errorf("%s: only %.1f%% of questions have labels (expected ≥10%%)", tag, labelRate*100)
	}
	if machineNameRate > 0.02 {
		t.Errorf("%s: %.1f%% of questions use machine-name fallback labels (expected ≤2%%)", tag, machineNameRate*100)
	}
	t.Logf("%s: %.1f%% of questions have labels (%d/%d)", tag, labelRate*100, labelledCount, len(schema.Questions))

	// 2. Section tree: the root section must have children.
	if len(schema.Sections) == 0 {
		t.Errorf("%s: no sections in schema", tag)
		return
	}
	rootSec := schema.Sections[0]
	if len(rootSec.Children) == 0 {
		t.Errorf("%s: root section has no children — section tree not built", tag)
	} else {
		t.Logf("%s: root has %d top-level sections", tag, len(rootSec.Children))
	}

	// 3. Section label quality: no section label should be >300 chars
	//    (that would indicate instruction text leaking into the label).
	longLabelCount := 0
	for _, child := range rootSec.Children {
		checkSectionLabels(t, tag, child, &longLabelCount)
	}
	if longLabelCount > 0 {
		t.Errorf("%s: %d section(s) have labels >300 chars — instruction text may be leaking into labels", tag, longLabelCount)
	}

	// 4. IMDRF TOC refs must not appear as section labels.
	//    They should be tooltips only (e.g. "IMDRF TOC CH3.05").
	imdrfInLabelCount := 0
	for _, child := range rootSec.Children {
		checkNoIMDRFInLabel(t, tag, child, &imdrfInLabelCount)
	}
	if imdrfInLabelCount > 0 {
		t.Errorf("%s: %d section label(s) contain IMDRF TOC references — these should be tooltips, not labels", tag, imdrfInLabelCount)
	}
}

func checkSectionLabels(t *testing.T, tag string, sec pdftypes.FormSection, count *int) {
	t.Helper()
	if len(sec.Label) > 300 {
		t.Logf("%s: section %q has label length %d: %.80s…", tag, sec.Name, len(sec.Label), sec.Label)
		*count++
	}
	for _, child := range sec.Children {
		checkSectionLabels(t, tag, child, count)
	}
}

func checkNoIMDRFInLabel(t *testing.T, tag string, sec pdftypes.FormSection, count *int) {
	t.Helper()
	if len(sec.Label) > 0 && bytes.Contains([]byte(sec.Label), []byte("IMDRF TOC")) {
		t.Logf("%s: section %q has IMDRF ref in label: %q", tag, sec.Name, sec.Label)
		*count++
	}
	for _, child := range sec.Children {
		checkNoIMDRFInLabel(t, tag, child, count)
	}
}

func assertFillWorks(t *testing.T, pdfBytes []byte, filename string) {
	t.Helper()
	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm(%s): %v", filename, err)
	}

	schema := form.Schema()
	// Fill the first five writable text fields with test data.
	fillData := make(pdfer.FormData)
	count := 0
	for _, q := range schema.Questions {
		if q.Type == pdftypes.ResponseTypeText && !q.ReadOnly && !q.Hidden {
			fillData[q.ID] = "Test Value"
			count++
			if count >= 5 {
				break
			}
		}
	}

	filled, err := form.Fill(pdfBytes, fillData, nil, false)
	if err != nil {
		t.Fatalf("Fill(%s): %v", filename, err)
	}
	if len(filled) == 0 {
		t.Fatalf("Fill(%s) returned empty PDF", filename)
	}
	if !bytes.HasPrefix(filled, []byte("%PDF-")) {
		t.Errorf("Fill(%s) output does not start with %%PDF- header", filename)
	}

	// Round-trip: re-extracting the form from the filled output must succeed
	// and yield a schema of comparable size. This catches cross-reference
	// table corruption (xref offsets pointing into stream content) and stream
	// dictionary corruption (lost /Filter entries) that earlier broke Adobe
	// without showing up in a %PDF- prefix check.
	refilled, err := pdfer.ExtractForm(filled, nil, false)
	if err != nil {
		t.Fatalf("Fill(%s) output is not re-parseable: %v", filename, err)
	}
	refilledSchema := refilled.Schema()
	if refilledSchema == nil {
		t.Fatalf("Fill(%s) output has nil schema on re-extract", filename)
	}
	if len(refilledSchema.Questions) < len(schema.Questions)*9/10 {
		t.Errorf("Fill(%s): re-extracted schema has %d questions, expected ~%d (lost more than 10%% — likely structural corruption)",
			filename, len(refilledSchema.Questions), len(schema.Questions))
	}
	t.Logf("%s fill: %d fields set, output %d bytes, re-extracted %d questions", filename, count, len(filled), len(refilledSchema.Questions))
}

// TestESTAR_NonIVD_Fill_PreservesFileID checks that filling an encrypted
// eSTAR carries the source's /ID[0] into the rewritten output trailer.
// /ID[0] is the PDF 1.7 §14.4 permanent identifier — meant to stay stable
// across edits — and downstream consumers (signatures, dedup, file
// tracking) sometimes key on it. We're emitting a decrypted view of the
// same logical document, so the permanent ID should carry over.
func TestESTAR_NonIVD_Fill_PreservesFileID(t *testing.T) {
	pdfBytes := readESTARPDF(t, "nonivd_estar.pdf")

	srcID := extractFirstFileID(t, pdfBytes)
	if len(srcID) == 0 {
		t.Fatal("source eSTAR has no /ID[0] in trailer — test premise broken")
	}

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm: %v", err)
	}
	filled, err := form.Fill(pdfBytes, pdfer.FormData{}, nil, false)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	outID := extractFirstFileID(t, filled)
	if len(outID) == 0 {
		t.Fatal("fill output has no /ID in trailer — file-ID preservation regressed")
	}
	if !bytes.Equal(outID, srcID) {
		t.Fatalf("source /ID[0] %x not preserved; output /ID[0] = %x", srcID, outID)
	}
}

// extractFirstFileID returns /ID[0] from the active trailer, where readers
// consult it. Handles both PDF formats:
//
//   - Classical trailer (PDF 1.4 style): `trailer << ... /ID [<..><..>] ... >>`
//     before `startxref`.
//   - Cross-reference stream (PDF 1.5+): no `trailer` keyword; the trailer
//     dict lives inside the /Type/XRef object referenced by the final
//     `startxref` offset, and /ID is in that object's dict.
//
// Returns nil if neither location has /ID. Critically, we do NOT scan the
// whole file: an orphan xref-stream object kept in the body (which pdfer's
// current decrypt-rewrite leaves behind) carries a stale /ID in its dict.
// A whole-file search would treat that as success even though PDF readers
// can't see it (they only consult the active trailer).
func extractFirstFileID(t *testing.T, pdfBytes []byte) []byte {
	t.Helper()
	idRE := regexp.MustCompile(`/ID\s*\[\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\]`)

	// Try classical trailer first.
	if ti := bytes.LastIndex(pdfBytes, []byte("trailer")); ti >= 0 {
		end := bytes.Index(pdfBytes[ti:], []byte("startxref"))
		if end < 0 {
			end = len(pdfBytes) - ti
		}
		if m := idRE.FindSubmatch(pdfBytes[ti : ti+end]); m != nil {
			return hexDecode(m[1])
		}
	}

	// Fall back to xref-stream layout: find the active xref via startxref
	// offset, parse the object's dict (everything before `stream`).
	sxIdx := bytes.LastIndex(pdfBytes, []byte("startxref"))
	if sxIdx < 0 {
		return nil
	}
	tail := pdfBytes[sxIdx+len("startxref"):]
	offRE := regexp.MustCompile(`^\s*(\d+)`)
	mo := offRE.FindSubmatch(tail)
	if mo == nil {
		return nil
	}
	var off int
	fmt.Sscanf(string(mo[1]), "%d", &off)
	if off < 0 || off >= len(pdfBytes) {
		return nil
	}
	// Read the xref-stream object dict (up to the `stream` keyword).
	endobj := bytes.Index(pdfBytes[off:], []byte("endobj"))
	if endobj < 0 {
		return nil
	}
	objBody := pdfBytes[off : off+endobj]
	dictEnd := bytes.Index(objBody, []byte("stream"))
	if dictEnd < 0 {
		dictEnd = len(objBody)
	}
	if m := idRE.FindSubmatch(objBody[:dictEnd]); m != nil {
		return hexDecode(m[1])
	}
	return nil
}

func hexDecode(s []byte) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		fmt.Sscanf(string(s[i*2:i*2+2]), "%02x", &b)
		out[i] = b
	}
	return out
}

// TestESTAR_NonIVD_Fill_DatasetsStreamFilterMatchesContent guards against a
// past Adobe-only regression where Fill rewrote the XFA datasets stream as
// plaintext XML while leaving /Filter/FlateDecode in the stream dict. The
// output reparsed fine in pdfer (which treats /Filter as a hint and
// fall-through-decodes), but Adobe Reader / Acrobat tried to inflate the
// plaintext, failed, and got stuck on the dynamic XFA "Please wait..."
// wrapper page indefinitely. The fix: compress on write iff the dict says
// FlateDecode. The test must look at the *raw* bytes between stream /
// endstream — not at the data Extract returns — because Extract silently
// inflates either way.
func TestESTAR_NonIVD_Fill_DatasetsStreamFilterMatchesContent(t *testing.T) {
	pdfBytes := readESTARPDF(t, "nonivd_estar.pdf")

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm: %v", err)
	}

	// Empty data exercises the exact "no-change round-trip" path the user
	// hit in production.
	filled, err := form.Fill(pdfBytes, pdfer.FormData{}, nil, false)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}

	dict, raw := findDatasetsRawStream(t, filled)
	if !bytes.Contains(dict, []byte("/FlateDecode")) {
		t.Fatalf("expected /FlateDecode in datasets stream dict (test premise broken), got: %s", string(dict))
	}
	// Strict: PDF /FlateDecode is RFC 1950 zlib (header + deflate + Adler-32),
	// not RFC 1951 raw deflate. Acrobat enforces this and hangs on the
	// "Please wait..." page when the bytes are raw deflate; Foxit silently
	// falls back. zlib.NewReader returning an error here would mean we've
	// regressed to raw-deflate output.
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("datasets /FlateDecode stream is not valid zlib (likely raw deflate — Acrobat will reject): %v\nfirst 8 bytes: % x",
			err, raw[:min(8, len(raw))])
	}
	if _, err := io.ReadAll(zr); err != nil {
		zr.Close()
		t.Fatalf("zlib decode of datasets stream failed: %v", err)
	}
	zr.Close()
}

// findDatasetsRawStream locates the object that holds the XFA datasets packet
// and returns its raw (still-encoded) dict and stream bytes. We have to walk
// the XFA array in AcroForm rather than guessing object numbers because Fill
// is free to renumber.
func findDatasetsRawStream(t *testing.T, pdfBytes []byte) (dict, stream []byte) {
	t.Helper()
	xfaArrRE := regexp.MustCompile(`/XFA\s*\[([^\]]*)\]`)
	arr := xfaArrRE.FindSubmatch(pdfBytes)
	if arr == nil {
		t.Fatalf("XFA array not found in output")
	}
	pairRE := regexp.MustCompile(`\((datasets)\)\s*(\d+)\s+(\d+)\s+R`)
	pair := pairRE.FindSubmatch(arr[1])
	if pair == nil {
		t.Fatalf("datasets entry not found in XFA array: %s", string(arr[1]))
	}
	objNum, _ := strconv.Atoi(string(pair[2]))
	headerRE := regexp.MustCompile(`(?m)^` + strconv.Itoa(objNum) + `\s+\d+\s+obj`)
	hdr := headerRE.FindIndex(pdfBytes)
	if hdr == nil {
		t.Fatalf("object header for datasets obj %d not found", objNum)
	}
	streamKW := bytes.Index(pdfBytes[hdr[0]:], []byte("stream"))
	if streamKW < 0 {
		t.Fatalf("stream keyword not found for datasets obj %d", objNum)
	}
	dict = pdfBytes[hdr[0] : hdr[0]+streamKW]
	dataStart := hdr[0] + streamKW + len("stream")
	for dataStart < len(pdfBytes) && (pdfBytes[dataStart] == '\r' || pdfBytes[dataStart] == '\n') {
		dataStart++
	}
	endKW := bytes.Index(pdfBytes[dataStart:], []byte("endstream"))
	if endKW < 0 {
		t.Fatalf("endstream not found for datasets obj %d", objNum)
	}
	streamEnd := dataStart + endKW
	for streamEnd > dataStart && (pdfBytes[streamEnd-1] == '\r' || pdfBytes[streamEnd-1] == '\n') {
		streamEnd--
	}
	return dict, pdfBytes[dataStart:streamEnd]
}

// readESTARPDF loads an eSTAR fixture PDF from tests/resources/ and skips
// if the file is not present.
func readESTARPDF(t *testing.T, filename string) []byte {
	t.Helper()
	path := getTestResourcePath(filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not found at %s — add it to tests/resources/ to run this test", filename, path)
	}
	return data
}
