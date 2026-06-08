// Package forms provides a unified interface for working with PDF forms
// It supports both AcroForm and XFA form types with automatic detection
package forms

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
	"github.com/benedoc-inc/pdfer/v2/forms/acroform"
	"github.com/benedoc-inc/pdfer/v2/forms/xfa"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// FormType represents the type of form
type FormType string

const (
	FormTypeAcroForm FormType = "acroform"
	FormTypeXFA      FormType = "xfa"
	FormTypeUnknown  FormType = "unknown"
)

// Form represents a unified form interface
type Form interface {
	// Type returns the form type (AcroForm or XFA)
	Type() FormType

	// Schema returns the form schema (structure and fields)
	Schema() *types.FormSchema

	// Fill fills the form with the provided data and returns modified PDF bytes
	Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error)

	// Validate validates form data against the form's validation rules
	Validate(data types.FormData) []error

	// GetValues returns the current values of all form fields
	GetValues() map[string]interface{}
}

// AcroFormWrapper wraps an AcroForm to implement the Form interface
type AcroFormWrapper struct {
	acroForm *acroform.AcroForm
	pdfBytes []byte
	password []byte
}

func (w *AcroFormWrapper) Type() FormType {
	return FormTypeAcroForm
}

func (w *AcroFormWrapper) Schema() *types.FormSchema {
	return w.acroForm.ToFormSchema()
}

func (w *AcroFormWrapper) Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error) {
	return acroform.FillFormFieldsWithStreams(pdfBytes, data, password, verbose)
}

func (w *AcroFormWrapper) Validate(data types.FormData) []error {
	return acroform.ValidateFormData(w.acroForm, data)
}

func (w *AcroFormWrapper) GetValues() map[string]interface{} {
	return w.acroForm.GetFieldValues()
}

// XFAFormWrapper wraps XFA form data to implement the Form interface
type XFAFormWrapper struct {
	formSchema     *types.FormSchema
	datasets       *types.XFADatasets
	pdfBytes       []byte
	password       []byte
	decryptedBytes []byte // non-nil when pdfBytes was encrypted; used by Fill
}

func (w *XFAFormWrapper) Type() FormType {
	return FormTypeXFA
}

func (w *XFAFormWrapper) Schema() *types.FormSchema {
	return w.formSchema
}

func (w *XFAFormWrapper) Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error) {
	// If we have pre-decrypted bytes (from encrypted PDF extraction), use them.
	// The XFA update pipeline requires plaintext object streams; passing the
	// original encrypted bytes causes object-stream decompression to fail.
	if w.decryptedBytes != nil {
		return xfa.UpdateXFAInPDF(w.decryptedBytes, data, nil, verbose)
	}

	// PDF was not encrypted at extraction time — but the caller may have supplied
	// a password for a separately-obtained encrypted copy.
	if len(password) > 0 || len(w.password) > 0 {
		pwd := password
		if len(pwd) == 0 {
			pwd = w.password
		}
		if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
			if decrypted, decErr := decryptForXFA(pdfBytes, pwd, verbose); decErr == nil {
				return xfa.UpdateXFAInPDF(decrypted, data, nil, verbose)
			}
		}
	}
	return xfa.UpdateXFAInPDF(pdfBytes, data, nil, verbose)
}

func (w *XFAFormWrapper) Validate(data types.FormData) []error {
	return []error{fmt.Errorf("XFA form validation is not implemented")}
}

func (w *XFAFormWrapper) GetValues() map[string]interface{} {
	if w.datasets != nil {
		return w.datasets.Fields
	}
	return make(map[string]interface{})
}

// Detect detects the form type in a PDF
func Detect(pdfBytes []byte, password []byte, verbose bool) (FormType, error) {
	// Try AcroForm first
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, verbose)
	if err == nil && acroForm != nil && len(acroForm.Fields) > 0 {
		return FormTypeAcroForm, nil
	}

	// Try XFA
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, nil, verbose)
	if err == nil && streams.Template != nil && len(streams.Template.Data) > 0 {
		return FormTypeXFA, nil
	}

	return FormTypeUnknown, types.NewPDFError(types.ErrCodeNoForms, "no forms detected in PDF")
}

// Extract extracts and returns a unified Form interface
// It automatically detects whether the PDF contains AcroForm or XFA forms
func Extract(pdfBytes []byte, password []byte, verbose bool) (Form, error) {
	// Try AcroForm first
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, verbose)
	if err == nil && acroForm != nil && len(acroForm.Fields) > 0 {
		return &AcroFormWrapper{
			acroForm: acroForm,
			pdfBytes: pdfBytes,
			password: password,
		}, nil
	}

	// Try XFA — decrypt first if the PDF is encrypted, since XFA stream
	// extraction requires plaintext bytes (encryptInfo=nil path).
	xfaBytes := pdfBytes
	var decryptedForFill []byte // non-nil if we decrypted pdfBytes
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		pwd := password
		if len(pwd) == 0 {
			pwd = []byte("")
		}
		if decrypted, decErr := decryptForXFA(pdfBytes, pwd, verbose); decErr == nil {
			xfaBytes = decrypted
			decryptedForFill = decrypted
		}
	}
	streams, err := xfa.ExtractAllXFAStreams(xfaBytes, nil, verbose)
	if err == nil && streams.Template != nil && len(streams.Template.Data) > 0 {
		// Build a resource map from any extra XFA packet streams (images, fonts, etc.)
		var resources map[string][]byte
		if len(streams.Resources) > 0 {
			resources = make(map[string][]byte, len(streams.Resources))
			for name, info := range streams.Resources {
				if info != nil {
					resources[name] = info.Data
				}
			}
		}
		// Parse XFA form, resolving any $rr: image href references.
		formSchema, err := xfa.ParseXFAFormWithResources(string(streams.Template.Data), resources, verbose)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInvalidForm, "failed to parse XFA form", err)
		}

		var datasets *types.XFADatasets
		if streams.Datasets != nil {
			datasets, _ = xfa.ParseXFADatasets(string(streams.Datasets.Data), verbose)
		}
		return &XFAFormWrapper{
			formSchema:     formSchema,
			datasets:       datasets,
			pdfBytes:       pdfBytes,
			password:       password,
			decryptedBytes: decryptedForFill,
		}, nil
	}

	return nil, types.NewPDFError(types.ErrCodeNoForms, "no forms found in PDF")
}

// ExtractAcroForm extracts an AcroForm (type-specific)
func ExtractAcroForm(pdfBytes []byte, password []byte, verbose bool) (*acroform.AcroForm, error) {
	return acroform.ExtractAcroForm(pdfBytes, password, verbose)
}

// ExtractXFA extracts XFA form data (type-specific)
// Returns the FormSchema and Datasets separately
func ExtractXFA(pdfBytes []byte, password []byte, verbose bool) (*types.FormSchema, *types.XFADatasets, error) {
	xfaBytes := pdfBytes
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		pwd := password
		if len(pwd) == 0 {
			pwd = []byte("")
		}
		if decrypted, decErr := decryptForXFA(pdfBytes, pwd, verbose); decErr == nil {
			xfaBytes = decrypted
		}
	}
	streams, err := xfa.ExtractAllXFAStreams(xfaBytes, nil, verbose)
	if err != nil {
		return nil, nil, err
	}

	var formSchema *types.FormSchema
	var datasets *types.XFADatasets

	if streams.Template != nil {
		var resources map[string][]byte
		if len(streams.Resources) > 0 {
			resources = make(map[string][]byte, len(streams.Resources))
			for name, info := range streams.Resources {
				if info != nil {
					resources[name] = info.Data
				}
			}
		}
		formSchema, _ = xfa.ParseXFAFormWithResources(string(streams.Template.Data), resources, verbose)
	}
	if streams.Datasets != nil {
		datasets, _ = xfa.ParseXFADatasets(string(streams.Datasets.Data), verbose)
	}

	return formSchema, datasets, nil
}

// decryptForXFA decrypts an encrypted PDF so that XFA stream extraction can
// operate on plaintext bytes. Uses parse + write directly to avoid an import
// cycle with core/manipulate (which imports forms via content/extract).
func decryptForXFA(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	if !pdf.IsEncrypted() {
		return pdfBytes, nil
	}
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("decrypt: no document catalog in trailer")
	}
	var rootNum, encryptObjNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)
	if trailer.EncryptRef != "" {
		fmt.Sscanf(trailer.EncryptRef, "%d", &encryptObjNum)
	}
	w := write.NewPDFWriter()
	for _, n := range pdf.Objects() {
		if n == encryptObjNum {
			continue
		}
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		if n == rootNum {
			body = xfaRemoveEncryptRef(body)
		}
		// /Filter[/Crypt] tells readers to decrypt the stream with the
		// document encryption key. After we've decrypted the bytes and
		// dropped /Encrypt, leaving /Crypt in the filter list makes
		// readers reject the stream ("Can't revert non decrypt streams").
		// Strip it from the dict portion before re-emitting.
		stripped, stripErr := xfaStripCryptInDict(body)
		if stripErr != nil {
			return nil, fmt.Errorf("decrypt: obj %d: %w", n, stripErr)
		}
		body = stripped
		dictBytes, streamBytes := xfaSplitContent(body)
		if streamBytes != nil {
			w.SetRawStreamObject(n, dictBytes, streamBytes)
		} else {
			w.SetObject(n, dictBytes)
		}
	}
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}
	// Preserve the source file's /ID[0] in the output trailer. PDF 1.5+
	// inputs (including eSTAR) keep their /ID inside the cross-reference
	// stream's dict, not a classical `trailer` block. PDFWriter emits a
	// classical trailer over direct objects, so without explicit
	// preservation here readers see no /ID at all — even though the source
	// had one. Per PDF 1.7 §14.4, /ID[0] is the permanent identifier and
	// should remain stable across edits of the same logical document, so
	// downstream tools (signatures, dedup, asset tracking) that key on it
	// keep working. We don't have a clean way to derive a new /ID[1] (the
	// "modified" identifier), so SetEncryption duplicates /ID[0] into both
	// slots — the convention the spec suggests when /ID[1] is unknown.
	//
	// Not required for Acrobat or Foxit to render — they ignore /ID — but
	// it's a real ecosystem regression for tools downstream of pdfer.
	if fileID := encrypt.ExtractFileID(pdfBytes, verbose); len(fileID) > 0 {
		w.SetEncryption(nil, fileID)
	}
	return w.Bytes()
}

var xfaEncryptRefRE = regexp.MustCompile(`/Encrypt\s+\d+\s+\d+\s+R`)

func xfaRemoveEncryptRef(b []byte) []byte {
	return xfaEncryptRefRE.ReplaceAll(b, nil)
}

// Matches the /Filter entry of a stream dict, in any of these forms:
//
//	/Filter/Crypt                         (bare name, only /Crypt)
//	/Filter [/Crypt]                      (single-element array)
//	/Filter [/Crypt /FlateDecode ...]     (array with /Crypt plus codecs)
//
// We don't try to handle the bare-name + extra-filters case because PDF syntax
// requires an array there — a bare /Filter only ever names one filter.
var xfaFilterRE = regexp.MustCompile(`/Filter\s*(/Crypt\b|\[[^\]]*\])`)

// xfaStripCryptInDict locates the dict portion of an object body (everything
// before the "stream" keyword, or the whole body for non-stream objects) and
// applies xfaStripCryptFilter to it. Scoping the regex to the dict portion
// avoids matching byte sequences that happen to spell "/Filter[/Crypt]"
// inside binary stream data.
func xfaStripCryptInDict(body []byte) ([]byte, error) {
	dictEnd := len(body)
	if i := bytes.Index(body, []byte("stream")); i >= 0 {
		dictEnd = i
	}
	stripped, err := xfaStripCryptFilter(body[:dictEnd])
	if err != nil {
		return nil, err
	}
	if len(stripped) == dictEnd {
		// No change — return original to avoid allocation.
		return body, nil
	}
	out := make([]byte, 0, len(stripped)+(len(body)-dictEnd))
	out = append(out, stripped...)
	out = append(out, body[dictEnd:]...)
	return out, nil
}

// ErrCryptFilterDecodeParmsConflict is returned by xfaStripCryptFilter when a
// stream dict contains both /Filter[/Crypt /Other...] and an array-form
// /DecodeParms entry. Per PDF 7.4.1 the two arrays are positionally aligned,
// so dropping /Crypt from /Filter would shift the parametrisation of every
// remaining filter by one — a bug we can't fix without also rewriting the
// /DecodeParms array. The eSTAR templates this code targets never combine
// /Crypt with other filters, so this case is unreachable in practice; we
// error rather than silently miscoding it for future inputs.
var ErrCryptFilterDecodeParmsConflict = errors.New("stream dict has /Filter array with /Crypt plus other filters and an array-form /DecodeParms; dropping /Crypt would misalign /DecodeParms (not supported)")

// xfaStripCryptFilter removes /Crypt from a stream dict's /Filter entry now
// that the stream bytes are plaintext (and /Encrypt is gone from the trailer).
// Returns dictBytes unchanged when /Filter contains no /Crypt.
//
// Returns ErrCryptFilterDecodeParmsConflict when /Crypt is one of several
// entries in a /Filter array and the dict also has an array-form
// /DecodeParms — we'd need to drop the matching /DecodeParms slot too, which
// this implementation doesn't do.
func xfaStripCryptFilter(dictBytes []byte) ([]byte, error) {
	m := xfaFilterRE.FindSubmatchIndex(dictBytes)
	if m == nil {
		return dictBytes, nil
	}
	value := dictBytes[m[2]:m[3]]
	if bytes.Equal(bytes.TrimSpace(value), []byte("/Crypt")) {
		// Bare-name form: drop the whole /Filter entry. Any /DecodeParms
		// becomes an orphan but that's not a correctness hazard — readers
		// ignore /DecodeParms without a corresponding /Filter.
		return append(append([]byte{}, dictBytes[:m[0]]...), dictBytes[m[1]:]...), nil
	}
	// Array form: split contents, drop any /Crypt name.
	if value[0] != '[' {
		return dictBytes, nil
	}
	inner := value[1 : len(value)-1]
	kept := make([][]byte, 0)
	for _, name := range bytes.Fields(inner) {
		if !bytes.Equal(name, []byte("/Crypt")) {
			kept = append(kept, name)
		}
	}
	if len(kept) > 0 && xfaDecodeParmsArrayRE.Match(dictBytes) {
		// /Crypt + other filters + array-form /DecodeParms = positional
		// misalignment after we drop /Crypt. Refuse rather than ship a
		// stream whose parametrisation has shifted by one slot.
		return nil, ErrCryptFilterDecodeParmsConflict
	}
	var replacement []byte
	switch len(kept) {
	case 0:
		// /Filter[/Crypt] — drop /Filter entirely.
		replacement = nil
	case 1:
		// Single remaining filter: emit as bare name (more compatible).
		replacement = append([]byte("/Filter"), kept[0]...)
	default:
		var buf bytes.Buffer
		buf.WriteString("/Filter[")
		for i, name := range kept {
			if i > 0 {
				buf.WriteByte(' ')
			}
			buf.Write(name)
		}
		buf.WriteByte(']')
		replacement = buf.Bytes()
	}
	out := make([]byte, 0, len(dictBytes))
	out = append(out, dictBytes[:m[0]]...)
	out = append(out, replacement...)
	out = append(out, dictBytes[m[1]:]...)
	return out, nil
}

// xfaDecodeParmsArrayRE matches an array-form /DecodeParms entry, which is
// the case where positional alignment with /Filter matters. A dict-form
// (`/DecodeParms <<...>>`) implies a single filter so /Crypt arithmetic
// doesn't apply.
var xfaDecodeParmsArrayRE = regexp.MustCompile(`/DecodeParms\s*\[`)

func xfaSplitContent(content []byte) (dictBytes, streamBytes []byte) {
	for _, sep := range [][]byte{[]byte("\nstream\r\n"), []byte("\nstream\n")} {
		idx := bytes.Index(content, sep)
		if idx < 0 {
			continue
		}
		dict := content[:idx]
		rest := content[idx+len(sep):]
		for _, end := range [][]byte{[]byte("\nendstream"), []byte("endstream")} {
			if j := bytes.Index(rest, end); j >= 0 {
				return dict, rest[:j]
			}
		}
	}
	return content, nil
}
