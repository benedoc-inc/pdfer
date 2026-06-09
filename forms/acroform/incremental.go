package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/incremental"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/types"
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
// Encrypted PDFs fall back to the legacy splice path: incremental filling relies
// on ParseAcroForm, which cannot open a password-protected document (it parses
// with an empty password internally). The appended objects are still written via
// the shared incremental writer, which carries the document /ID forward.
func fillIncremental(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// Encrypted PDFs: fall back to the encryption-aware splice path.
	if pdf.Encryption() != nil {
		return FillFormFieldsWithStreams(pdfBytes, formData, password, verbose)
	}

	acroForm, err := ParseAcroForm(pdfBytes, nil, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}

	// ---- Resolve fields ----------------------------------------------------

	type fieldFill struct {
		field     *Field
		value     interface{}
		current   []byte // original object bytes
		xobjNum   int    // >0 = text/choice AP stream object number
		btnYesObj int    // >0 = generate "checked" checkbox AP XObject
		btnOffObj int    // >0 = generate "unchecked" checkbox AP XObject
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
		current, err := pdf.GetObjectContent(field.ObjectNum)
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

	// ---- Build incremental update ------------------------------------------
	//
	// Fresh object numbers (starting at trailer.Size) are reserved for:
	//   • one shared Helvetica font resource (if any text/choice field is filled)
	//   • one form XObject per text/choice field (the appearance stream)
	//   • a pair of AP XObjects for checkboxes/radios lacking an existing /AP

	upd, err := incremental.New(pdfBytes, trailer, pdf.Encryption())
	if err != nil {
		return nil, err
	}

	var helveticaObjNum int
	if needsFont {
		helveticaObjNum = upd.Reserve()
	}

	for i := range fills {
		switch {
		case fills[i].field.FT == "Tx" || fills[i].field.FT == "Ch":
			fills[i].xobjNum = upd.Reserve()
		case fills[i].field.FT == "Btn" && !isBtnPushButton(fills[i].field.Ff):
			if !bytes.Contains(fills[i].current, []byte("/AP")) {
				fills[i].btnYesObj = upd.Reserve()
				fills[i].btnOffObj = upd.Reserve()
			}
		}
	}

	// Shared Helvetica font object (no strings, no stream).
	if needsFont {
		font := "<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>"
		if err := upd.AddObject(helveticaObjNum, 0, []byte(font)); err != nil {
			return nil, err
		}
	}

	for _, ff := range fills {
		// Text/choice appearance XObject.
		if ff.xobjNum > 0 {
			fontAlias, fontSize := parseDA(ff.field.DA)
			dictBytes, streamBytes := buildTextAppearance(
				ff.field, fmt.Sprint(ff.value),
				fontAlias, fontSize, helveticaObjNum,
			)
			if err := upd.AddStream(ff.xobjNum, 0, dictBytes, streamBytes); err != nil {
				return nil, err
			}
		}

		// Checkbox AP XObjects (only when field has no pre-existing /AP).
		if ff.btnYesObj > 0 {
			var w, h float64
			if len(ff.field.Rect) >= 4 {
				w = ff.field.Rect[2] - ff.field.Rect[0]
				h = ff.field.Rect[3] - ff.field.Rect[1]
			}
			yesDict, yesStream := buildCheckboxAPStream(w, h, true)
			offDict, offStream := buildCheckboxAPStream(w, h, false)
			if err := upd.AddStream(ff.btnYesObj, 0, yesDict, yesStream); err != nil {
				return nil, err
			}
			if err := upd.AddStream(ff.btnOffObj, 0, offDict, offStream); err != nil {
				return nil, err
			}
		}

		// Updated field dict — current is body-only (from GetObjectContent).
		newBody := applyFieldValue(ff.current, ff.field, ff.value)
		switch {
		case ff.xobjNum > 0:
			newBody = withAppearanceRef(newBody, ff.xobjNum)
		case ff.field.FT == "Btn" && !isBtnPushButton(ff.field.Ff) && len(ff.field.Kids) == 0:
			// Simple checkbox (no kids): set /AS to match /V.
			state := btnCheckboxState(ff.value)
			newBody = withAS(newBody, state)
			if ff.btnYesObj > 0 {
				newBody = withBtnAP(newBody, ff.btnYesObj, ff.btnOffObj)
			}
		}
		if err := upd.AddObject(ff.field.ObjectNum, ff.field.Generation, newBody); err != nil {
			return nil, err
		}

		// Radio button group: update each kid widget's /AS.
		if ff.field.FT == "Btn" && isBtnRadio(ff.field.Ff) && len(ff.field.Kids) > 0 {
			selectedVal := fmt.Sprint(ff.value)
			for _, kid := range ff.field.Kids {
				kidBody, err := pdf.GetObjectContent(kid.ObjectNum)
				if err != nil {
					continue
				}
				exportVal := btnKidExportValue(kidBody)
				kidAS := "Off"
				if exportVal != "" && exportVal == selectedVal {
					kidAS = exportVal
				}
				newKidBody := withAS(kidBody, kidAS)
				if err := upd.AddObject(kid.ObjectNum, kid.Generation, newKidBody); err != nil {
					return nil, err
				}
			}
		}
	}

	return upd.Bytes()
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
