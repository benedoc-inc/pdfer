package tests

import (
	"bytes"
	"os"
	"testing"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// assertNoStaleContainers fails if a rebuilt PDF still carries object-stream or
// cross-reference-stream containers. The default writer emits a classical xref
// over direct objects, so any /Type/ObjStm or /Type/XRef in the output is a
// stale container that the rebuild path forgot to drop (issue #26, Bug 2).
func assertNoStaleContainers(t *testing.T, out []byte, label string) {
	t.Helper()
	if n := bytes.Count(out, []byte("/Type/XRef")); n != 0 {
		t.Errorf("%s: %d stale /Type/XRef containers in output", label, n)
	}
	if n := bytes.Count(out, []byte("/Type/ObjStm")); n != 0 {
		t.Errorf("%s: %d stale /Type/ObjStm containers in output", label, n)
	}
}

// assertCatalogResolves re-parses a rebuilt PDF and asserts its catalog is
// reachable — the symptom of the round-trip corruption was an unresolvable
// catalog after a rebuild of a PDF 1.5+ source.
func assertCatalogResolves(t *testing.T, out []byte, password []byte, label string) *parse.PDF {
	t.Helper()
	pdf, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("%s: re-parse failed: %v", label, err)
	}
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		t.Fatalf("%s: no document catalog in trailer", label)
	}
	return pdf
}

// TestE2E_RoundtripObjStm guards against issue #26's second bug: round-trip data
// loss on PDF 1.5+ sources. The acroform_test.pdf fixture stores its catalog and
// AcroForm inside an object stream and ships a cross-reference stream. Rebuild
// paths (EncryptPDF, DecryptPDF, FlattenForm, PDF/A conversion, Split/Redact)
// flatten object-stream members into standalone objects and the writer
// regenerates a fresh classical xref.
//
// THE bug, and THE fix: parseTraditionalxrefSection truncated its read at a
// fixed 10000-byte window, silently dropping every object past ~entry 500 — so a
// rebuilt file with >500 objects re-parsed with an unresolvable catalog and
// zero /AcroForm. Reading the xref section in full fixes it; the catalog /
// object-875 / 32-field assertions below are the regression checks for that.
//
// The "no stale container" assertions are a SEPARATE concern — hygiene, not the
// fix. The rebuilders also used to re-emit the redundant /Type/ObjStm and
// /Type/XRef containers as orphan objects; dropping them keeps output clean but
// is not what makes the round trip succeed (verified: parser fix alone recovers
// all objects even with the orphans still present).
func TestE2E_RoundtripObjStm(t *testing.T) {
	src, err := os.ReadFile(getTestResourcePath("acroform_test.pdf"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// reparses asserts an acroform_test.pdf rebuild is structurally intact: the
	// catalog plus the AcroForm (875 0 R) and catalog (876 0 R) objects resolve,
	// and no stale cross-reference containers survived.
	reparses := func(t *testing.T, out []byte, password []byte, label string) {
		t.Helper()
		pdf := assertCatalogResolves(t, out, password, label)
		// Before the fix, re-parse recovered only ~498 of 877 objects, so these
		// high-numbered objects were unreachable and downstream /AcroForm was 0.
		if !pdf.HasObject(875) {
			t.Errorf("%s: object 875 (AcroForm) not resolvable after rebuild", label)
		}
		if _, err := pdf.GetObjectContent(876); err != nil {
			t.Errorf("%s: object 876 (catalog) not resolvable after rebuild: %v", label, err)
		}
		assertNoStaleContainers(t, out, label)
	}

	// Path B — the issue's reported failing case: encrypt then decrypt, then
	// extract the form. The data loss is already complete after EncryptPDF.
	t.Run("EncryptDecryptExtract", func(t *testing.T) {
		enc, err := pdfer.EncryptPDF(src, []byte("user-pw"), []byte("owner-pw"), false)
		if err != nil {
			t.Fatalf("EncryptPDF: %v", err)
		}
		reparses(t, enc, []byte("user-pw"), "encrypted")

		dec, err := pdfer.DecryptPDF(enc, []byte("user-pw"), false)
		if err != nil {
			t.Fatalf("DecryptPDF: %v", err)
		}
		reparses(t, dec, nil, "decrypted")

		// Issue reported zero /AcroForm in the decrypted output.
		if !bytes.Contains(dec, []byte("/AcroForm")) {
			t.Error("decrypted output is missing /AcroForm")
		}

		form, err := pdfer.ExtractForm(dec, nil, false)
		if err != nil {
			t.Fatalf("ExtractForm(decrypted): %v", err)
		}
		if form.Type() != pdfer.FormTypeAcroForm {
			t.Errorf("Type() = %q, want %q", form.Type(), pdfer.FormTypeAcroForm)
		}
		if got := len(form.Schema().Questions); got != 32 {
			t.Errorf("Schema().Questions = %d, want 32", got)
		}
	})

	// Other rebuild ops on the same 1.5+ source must also stay intact and drop
	// the containers.
	t.Run("FlattenForm", func(t *testing.T) {
		out, err := pdfer.FlattenForm(src, nil, false)
		if err != nil {
			t.Fatalf("FlattenForm: %v", err)
		}
		reparses(t, out, nil, "flattened")
	})

	t.Run("ConvertToPDFA", func(t *testing.T) {
		out, err := write.ConvertToPDFA(src, write.PDFA2b)
		if err != nil {
			t.Fatalf("ConvertToPDFA: %v", err)
		}
		reparses(t, out, nil, "pdfa")
	})

	t.Run("FixPDFA", func(t *testing.T) {
		out, err := write.FixPDFA(src, nil)
		if err != nil {
			t.Fatalf("FixPDFA: %v", err)
		}
		reparses(t, out, nil, "fixpdfa")
	})

	// Split and Redact go through PDFManipulator.Rebuild. acroform_test.pdf has a
	// page tree these ops cannot enumerate (a separate, pre-existing limitation),
	// so the Rebuild guard is exercised against fda_3881.pdf — also a PDF 1.5+
	// source with ObjStm + XRef containers.
	src17, err := os.ReadFile(getTestResourcePath("fda_3881.pdf"))
	if err != nil {
		t.Fatalf("read fda_3881 fixture: %v", err)
	}

	t.Run("Split", func(t *testing.T) {
		parts, err := pdfer.SplitPDF(src17, []pdfer.PageRange{{Start: 1, End: 1}}, nil, false)
		if err != nil {
			t.Fatalf("SplitPDF: %v", err)
		}
		if len(parts) == 0 {
			t.Fatal("SplitPDF returned no parts")
		}
		assertCatalogResolves(t, parts[0], nil, "split")
		assertNoStaleContainers(t, parts[0], "split")
	})

	t.Run("Redact", func(t *testing.T) {
		out, err := pdfer.Redact(src17, []pdfer.RedactBox{{Page: 1, Rect: [4]float64{50, 50, 200, 100}}}, nil)
		if err != nil {
			t.Fatalf("Redact: %v", err)
		}
		assertCatalogResolves(t, out, nil, "redacted")
		assertNoStaleContainers(t, out, "redacted")
	})
}
