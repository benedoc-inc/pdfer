// Package acroform provides integration with SimplePDFBuilder
package acroform

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/types"
	"github.com/benedoc-inc/pdfer/core/write"
)

// FormBuilder integrates AcroForm creation with SimplePDFBuilder
type FormBuilder struct {
	builder           *write.SimplePDFBuilder
	fieldBuilder      *FieldBuilder
	appearanceBuilder *AppearanceBuilder
}

// NewFormBuilder creates a new form builder integrated with SimplePDFBuilder
func NewFormBuilder(builder *write.SimplePDFBuilder) *FormBuilder {
	w := builder.Writer()
	return &FormBuilder{
		builder:           builder,
		fieldBuilder:      NewFieldBuilder(w),
		appearanceBuilder: NewAppearanceBuilder(w),
	}
}

// AddTextField adds a text field to the form
func (fb *FormBuilder) AddTextField(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddTextField(name, rect, page)
}

// AddCheckbox adds a checkbox field
func (fb *FormBuilder) AddCheckbox(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddCheckbox(name, rect, page)
}

// AddRadioButton adds a radio button field
func (fb *FormBuilder) AddRadioButton(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddRadioButton(name, rect, page)
}

// AddChoiceField adds a choice (dropdown/list) field
func (fb *FormBuilder) AddChoiceField(name string, rect []float64, page int, options []string) *FieldDef {
	return fb.fieldBuilder.AddChoiceField(name, rect, page, options)
}

// AddButton adds a push button
func (fb *FormBuilder) AddButton(name string, rect []float64, page int) *FieldDef {
	return fb.fieldBuilder.AddButton(name, rect, page)
}

// BuildForm builds the AcroForm and integrates it with the PDF
func (fb *FormBuilder) BuildForm() (int, error) {
	acroFormNum, err := fb.fieldBuilder.Build()
	if err != nil {
		return 0, fmt.Errorf("failed to build AcroForm: %w", err)
	}

	// Get pages object number
	pagesObjNum := fb.builder.PagesObjNum()

	// Update catalog to include AcroForm
	w := fb.builder.Writer()
	catalogDict := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>",
		pagesObjNum, acroFormNum)
	catalogNum := w.AddObject([]byte(catalogDict))
	w.SetRoot(catalogNum)

	return acroFormNum, nil
}

// FillFormFromSchema fills a form from FormSchema and FormData
func FillFormFromSchema(pdfBytes []byte, schema *types.FormSchema, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	// Validate form data first
	errors := ValidateFormSchema(schema, formData)
	if len(errors) > 0 {
		if verbose {
			for _, err := range errors {
				fmt.Printf("Validation error: %v\n", err)
			}
		}
		// Continue anyway - validation errors are warnings
	}

	// Fill the form
	return FillFormFields(pdfBytes, formData, password, verbose)
}

// ExtractAndFill extracts AcroForm, validates, and fills
func ExtractAndFill(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	// Extract AcroForm
	acroForm, err := ExtractAcroForm(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract AcroForm: %w", err)
	}

	// Validate
	errors := ValidateFormData(acroForm, formData)
	if len(errors) > 0 && verbose {
		for _, err := range errors {
			fmt.Printf("Validation warning: %v\n", err)
		}
	}

	// Fill
	return FillFormFields(pdfBytes, formData, password, verbose)
}
