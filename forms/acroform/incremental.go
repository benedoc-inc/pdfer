package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// fillIncremental fills AcroForm fields using a PDF incremental update.
//
// Rather than splicing bytes in-place (which shifts subsequent object offsets
// and corrupts the xref table), each modified object is re-written verbatim at
// the end of the original bytes. A new xref section and trailer referencing the
// old xref via /Prev are appended. Original bytes are never altered.
//
// Encrypted PDFs fall back to the legacy splice path because incremental
// updates for encrypted files require per-string key derivation that is not
// yet implemented here.
func fillIncremental(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// Encrypted PDFs: fall back to legacy path (encryption is out of scope here).
	if pdf.Encryption() != nil {
		return FillFormFieldsWithStreams(pdfBytes, formData, password, verbose)
	}

	acroForm, err := ParseAcroForm(pdfBytes, nil, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	// Build the list of (objectNum, updatedBytes) pairs.
	type update struct {
		objNum int
		genNum int
		body   []byte
	}
	var updates []update

	for fieldName, value := range formData {
		field := acroForm.FindFieldByName(fieldName)
		if field == nil {
			if verbose {
				fmt.Printf("Warning: field %q not found, skipping\n", fieldName)
			}
			continue
		}

		current, err := pdf.GetObject(field.ObjectNum)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: cannot read object %d for field %q: %v\n", field.ObjectNum, fieldName, err)
			}
			continue
		}

		newBody := applyFieldValue(current, field, value)
		updates = append(updates, update{field.ObjectNum, field.Generation, newBody})
	}

	if len(updates) == 0 {
		return pdfBytes, nil
	}

	// prevXRef is the byte offset stored after "startxref" in the original file;
	// it becomes /Prev in the new trailer.
	prevXRef := findStartXRef(pdfBytes)
	if prevXRef < 0 {
		return nil, fmt.Errorf("could not locate startxref in original PDF")
	}

	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}

	// --- Build incremental update section ---
	var buf bytes.Buffer
	buf.Write(pdfBytes)
	buf.WriteByte('\n')

	offsets := make(map[int]int64, len(updates))
	for _, upd := range updates {
		offsets[upd.objNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d %d obj\n", upd.objNum, upd.genNum)
		buf.Write(upd.body)
		buf.WriteString("\nendobj\n")
	}

	// xref table: group consecutive object numbers into subsections.
	xrefStart := int64(buf.Len())
	buf.WriteString("xref\n")
	objNums := make([]int, 0, len(offsets))
	for n := range offsets {
		objNums = append(objNums, n)
	}
	sort.Ints(objNums)
	writeXRefSubsections(&buf, offsets, objNums)

	// New trailer: preserve all original refs, add /Prev.
	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, "/Size %d", trailer.Size)
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

// applyFieldValue returns a copy of fieldData with the /V entry set to value.
// For button fields (checkboxes/radio) the value is written as a PDF name (/Yes,
// /Off, etc.); for all other field types it is written as a PDF string literal.
func applyFieldValue(fieldData []byte, field *Field, value interface{}) []byte {
	fieldStr := string(fieldData)
	valueStr := formatFieldValue(value, field.FT)

	var newV string
	if field.FT == "Btn" {
		// Checkbox / radio: value is a name, not a string.
		newV = fmt.Sprintf("/V/%s", valueStr)
	} else {
		newV = fmt.Sprintf("/V (%s)", escapeFieldValue(valueStr))
	}

	// Replace existing /V entry or insert before closing >>.
	vPat := regexp.MustCompile(`/V\s*(?:\([^)]*\)|/[^\s/>\[]+|\[[^\]]*\])`)
	var result string
	if vPat.MatchString(fieldStr) {
		result = vPat.ReplaceAllString(fieldStr, newV)
	} else {
		end := strings.LastIndex(fieldStr, ">>")
		if end == -1 {
			return fieldData
		}
		result = fieldStr[:end] + newV + " " + fieldStr[end:]
	}
	return []byte(result)
}
