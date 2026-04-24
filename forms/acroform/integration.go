package acroform

import (
	"fmt"
	"strings"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// FormBuilder integrates AcroForm creation with SimplePDFBuilder.
// Typical usage:
//
//	builder := write.NewSimplePDFBuilder()
//	page := builder.AddPage(write.PageSizeLetter)
//	// ... add page content ...
//	builder.FinalizePage(page)
//
//	fb := acroform.NewFormBuilder(builder)
//	fb.AddTextField("name", []float64{72, 700, 300, 720}, 0)
//	fb.BuildForm()
//
//	pdfBytes, _ := builder.Bytes()
type FormBuilder struct {
	builder      *write.SimplePDFBuilder
	fieldBuilder *FieldBuilder
}

// NewFormBuilder creates a new FormBuilder backed by the given SimplePDFBuilder.
func NewFormBuilder(builder *write.SimplePDFBuilder) *FormBuilder {
	return &FormBuilder{
		builder:      builder,
		fieldBuilder: NewFieldBuilder(builder.Writer()),
	}
}

func (fb *FormBuilder) AddTextField(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddTextField(name, rect, page)
}

func (fb *FormBuilder) AddCheckbox(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddCheckbox(name, rect, page)
}

func (fb *FormBuilder) AddRadioButton(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddRadioButton(name, rect, page)
}

func (fb *FormBuilder) AddChoiceField(name string, rect []float64, page int, options []string) *FieldDef {
	return fb.fieldBuilder.AddChoiceField(name, rect, page, options)
}

func (fb *FormBuilder) AddButton(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddButton(name, rect, page)
}

// BuildForm writes all field+widget objects, wires them into the correct page
// /Annots arrays, builds the /AcroForm dict, and registers it with the
// SimplePDFBuilder so Bytes() includes /AcroForm in the catalog.
//
// Call this AFTER all pages have been finalized with FinalizePage().
func (fb *FormBuilder) BuildForm() (int, error) {
	pageObjNums := fb.builder.Pages()
	if len(pageObjNums) == 0 {
		return 0, fmt.Errorf("no pages finalized; call FinalizePage before BuildForm")
	}

	acroFormNum, fieldsByPage, err := fb.fieldBuilder.buildWithPages(pageObjNums)
	if err != nil {
		return 0, fmt.Errorf("building AcroForm: %w", err)
	}

	// Add each field object number to the corresponding page's /Annots array.
	w := fb.builder.Writer()
	for pageIdx, fieldObjNums := range fieldsByPage {
		if pageIdx < 0 || pageIdx >= len(pageObjNums) {
			continue
		}
		if err := addAnnotsToPage(w, pageObjNums[pageIdx], fieldObjNums); err != nil {
			return 0, fmt.Errorf("adding annots to page %d: %w", pageIdx+1, err)
		}
	}

	// Register AcroForm so Bytes() includes it in the catalog.
	fb.builder.RegisterAcroForm(acroFormNum)

	return acroFormNum, nil
}

// addAnnotsToPage appends objNums to the /Annots array of the page dict at pageObjNum.
func addAnnotsToPage(w *write.PDFWriter, pageObjNum int, objNums []int) error {
	pageData, err := w.GetObject(pageObjNum)
	if err != nil {
		return err
	}
	pageStr := string(pageData)

	// Build the refs to add.
	var newRefs strings.Builder
	for _, n := range objNums {
		fmt.Fprintf(&newRefs, "%d 0 R ", n)
	}
	refs := strings.TrimSpace(newRefs.String())

	// If /Annots already exists, append inside the existing array.
	if idx := strings.Index(pageStr, "/Annots["); idx != -1 {
		insertAt := idx + len("/Annots[")
		pageStr = pageStr[:insertAt] + refs + " " + pageStr[insertAt:]
	} else if idx := strings.Index(pageStr, "/Annots ["); idx != -1 {
		insertAt := idx + len("/Annots [")
		pageStr = pageStr[:insertAt] + refs + " " + pageStr[insertAt:]
	} else {
		// No /Annots yet — add one before the closing >>.
		dictEnd := strings.LastIndex(pageStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("page dict has no closing >>")
		}
		pageStr = pageStr[:dictEnd] + "/Annots[" + refs + "]" + pageStr[dictEnd:]
	}

	w.SetObject(pageObjNum, []byte(pageStr))
	return nil
}

// FillFormFromSchema fills a form from FormSchema and FormData.
func FillFormFromSchema(pdfBytes []byte, schema *types.FormSchema, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	errs := ValidateFormSchema(schema, formData)
	if len(errs) > 0 && verbose {
		for _, e := range errs {
			fmt.Printf("Validation warning: %v\n", e)
		}
	}
	return FillFormFields(pdfBytes, formData, password, verbose)
}

// ExtractAndFill extracts AcroForm, validates, and fills.
func ExtractAndFill(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	acroForm, err := ExtractAcroForm(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract AcroForm: %w", err)
	}
	errs := ValidateFormData(acroForm, formData)
	if len(errs) > 0 && verbose {
		for _, e := range errs {
			fmt.Printf("Validation warning: %v\n", e)
		}
	}
	return FillFormFields(pdfBytes, formData, password, verbose)
}
