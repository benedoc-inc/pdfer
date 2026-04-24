package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
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
// For text and choice fields an /AP appearance stream is also generated so the
// filled value is visible in viewers that do not honour /NeedAppearances true.
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

	// Encrypted PDFs: fall back to legacy path.
	if pdf.Encryption() != nil {
		return FillFormFieldsWithStreams(pdfBytes, formData, password, verbose)
	}

	acroForm, err := ParseAcroForm(pdfBytes, nil, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	prevXRef := findStartXRef(pdfBytes)
	if prevXRef < 0 {
		return nil, fmt.Errorf("could not locate startxref in original PDF")
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}

	// ---- Resolve fields and pre-assign object numbers ----------------------
	//
	// We assign fresh object numbers (starting at trailer.Size) for:
	//   • one shared Helvetica font resource (if any text/choice field is filled)
	//   • one form XObject per text/choice field (the appearance stream)
	// These are appended to the PDF and referenced from the updated field dicts.

	type fieldFill struct {
		field   *Field
		value   interface{}
		current []byte // original object bytes
		xobjNum int    // 0 = no appearance needed (checkbox/radio/push-button)
	}

	var fills []fieldFill
	needsFont := false

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
		ff := fieldFill{field: field, value: value, current: current}
		if field.FT == "Tx" || field.FT == "Ch" {
			needsFont = true
		}
		fills = append(fills, ff)
	}

	if len(fills) == 0 {
		return pdfBytes, nil
	}

	nextObjNum := trailer.Size

	// Shared Helvetica font for appearance streams.
	var helveticaObjNum int
	if needsFont {
		helveticaObjNum = nextObjNum
		nextObjNum++
	}

	// Assign XObject numbers for text/choice fields.
	for i := range fills {
		if fills[i].field.FT == "Tx" || fills[i].field.FT == "Ch" {
			fills[i].xobjNum = nextObjNum
			nextObjNum++
		}
	}

	// ---- Build incremental update section ----------------------------------

	var buf bytes.Buffer
	buf.Write(pdfBytes)
	buf.WriteByte('\n')

	offsets := make(map[int]int64)

	// Shared Helvetica font object.
	if needsFont {
		offsets[helveticaObjNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", helveticaObjNum)
		buf.WriteString("<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>")
		buf.WriteString("\nendobj\n")
	}

	// Per-field appearance XObjects, then updated field dicts.
	for _, ff := range fills {
		// Appearance XObject (text and choice fields only).
		if ff.xobjNum > 0 {
			fontAlias, fontSize := parseDA(ff.field.DA)
			dictBytes, streamBytes := buildTextAppearance(
				ff.field, fmt.Sprint(ff.value),
				fontAlias, fontSize, helveticaObjNum,
			)
			offsets[ff.xobjNum] = int64(buf.Len())
			fmt.Fprintf(&buf, "%d 0 obj\n", ff.xobjNum)
			buf.Write(dictBytes)
			buf.WriteString("\nstream\n")
			buf.Write(streamBytes)
			buf.WriteString("endstream\nendobj\n")
		}

		// Updated field dict.
		newBody := applyFieldValue(ff.current, ff.field, ff.value)
		if ff.xobjNum > 0 {
			newBody = withAppearanceRef(newBody, ff.xobjNum)
		}
		offsets[ff.field.ObjectNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d %d obj\n", ff.field.ObjectNum, ff.field.Generation)
		buf.Write(newBody)
		buf.WriteString("\nendobj\n")
	}

	// xref table: consecutive runs become single subsections.
	xrefStart := int64(buf.Len())
	buf.WriteString("xref\n")
	objNums := make([]int, 0, len(offsets))
	for n := range offsets {
		objNums = append(objNums, n)
	}
	sort.Ints(objNums)
	writeXRefSubsections(&buf, offsets, objNums)

	// New trailer: preserve Root/Info/Encrypt, add Prev, update Size.
	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, "/Size %d", nextObjNum)
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

// buildTextAppearance returns the dict bytes and stream bytes for a text-field
// form XObject that renders value inside the field's bounding box.
func buildTextAppearance(field *Field, value, fontAlias string, fontSize float64, fontObjNum int) (dictBytes, streamBytes []byte) {
	var w, h float64
	if len(field.Rect) >= 4 {
		w = field.Rect[2] - field.Rect[0]
		h = field.Rect[3] - field.Rect[1]
	}
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 20
	}
	if fontSize <= 0 {
		fontSize = 12
	}

	// Vertical centering: place baseline so cap-height is centred.
	// Approximate cap-height as 0.7 × fontSize; descenders ≈ 0.2 × fontSize.
	td := (h - fontSize*0.7) / 2

	stream := fmt.Sprintf("/Tx BMC\nq\nBT\n/%s %.4g Tf\n0 g\n2 %.4g Td\n(%s) Tj\nET\nQ\nEMC\n",
		fontAlias, fontSize, td,
		escapeFieldValue(value),
	)

	dict := fmt.Sprintf(
		"<</Type/XObject/Subtype/Form/BBox[0 0 %.4g %.4g]/Resources<</Font<</%s %d 0 R>>>>/Length %d>>",
		w, h, fontAlias, fontObjNum, len(stream),
	)
	return []byte(dict), []byte(stream)
}

// parseDA extracts the font name alias and size from a /DA default-appearance
// string like "/Helv 12 Tf 0 g".  Returns safe defaults if parsing fails.
func parseDA(da string) (fontAlias string, fontSize float64) {
	fontAlias = "Helv"
	fontSize = 12

	parts := strings.Fields(da)
	for i, p := range parts {
		if strings.HasPrefix(p, "/") && i+2 < len(parts) && parts[i+2] == "Tf" {
			fontAlias = p[1:]
			if sz, err := strconv.ParseFloat(parts[i+1], 64); err == nil && sz > 0 {
				fontSize = sz
			}
			break
		}
	}
	return
}

// withAppearanceRef adds "/AP<</N xobjNum 0 R>>" to a field dict, replacing
// any pre-existing /AP entry.
func withAppearanceRef(fieldBody []byte, xobjNum int) []byte {
	fieldStr := string(fieldBody)
	newAP := fmt.Sprintf("/AP<</N %d 0 R>>", xobjNum)

	// Remove any existing /AP entry (handles simple one-level /AP dicts).
	apPat := regexp.MustCompile(`/AP\s*<<[^<>]*(?:<<[^<>]*>>[^<>]*)*>>`)
	if apPat.MatchString(fieldStr) {
		fieldStr = apPat.ReplaceAllString(fieldStr, "")
	}

	end := strings.LastIndex(fieldStr, ">>")
	if end == -1 {
		return fieldBody
	}
	return []byte(fieldStr[:end] + newAP + fieldStr[end:])
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
// Button fields (checkboxes/radio) use PDF name syntax (/Yes, /Off); all others
// use string literal syntax ((value)).
func applyFieldValue(fieldData []byte, field *Field, value interface{}) []byte {
	fieldStr := string(fieldData)
	valueStr := formatFieldValue(value, field.FT)

	var newV string
	if field.FT == "Btn" {
		newV = fmt.Sprintf("/V/%s", valueStr)
	} else {
		newV = fmt.Sprintf("/V (%s)", escapeFieldValue(valueStr))
	}

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
