package compare

import (
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/acroform"
)

func TestCompareForms_NoForms(t *testing.T) {
	// Create two PDFs without forms
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Hello World")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Hello World")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	// Should have no form differences
	if result.FormDiff != nil {
		t.Error("Expected no form differences for PDFs without forms")
	}
}

func TestCompareForms_FormFieldValueChange(t *testing.T) {
	// Create PDF with form field
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	fieldBuilder1 := acroform.NewFieldBuilder(builder1.Writer())
	field1 := fieldBuilder1.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field1.SetValue("John")
	builder1.FinalizePage(page1)
	acroFormNum1, _ := fieldBuilder1.Build()
	w1 := builder1.Writer()
	pagesObjNum1 := builder1.PagesObjNum()
	catalogDict1 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum1, acroFormNum1)
	catalogNum1 := w1.AddObject([]byte(catalogDict1))
	w1.SetRoot(catalogNum1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with same form but different value
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	fieldBuilder2 := acroform.NewFieldBuilder(builder2.Writer())
	field2 := fieldBuilder2.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field2.SetValue("Jane")
	builder2.FinalizePage(page2)
	acroFormNum2, _ := fieldBuilder2.Build()
	w2 := builder2.Writer()
	pagesObjNum2 := builder2.PagesObjNum()
	catalogDict2 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum2, acroFormNum2)
	catalogNum2 := w2.AddObject([]byte(catalogDict2))
	w2.SetRoot(catalogNum2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if result.FormDiff == nil {
		t.Fatal("Expected form differences")
	}

	if len(result.FormDiff.Modified) != 1 {
		t.Errorf("Expected 1 modified field, got %d", len(result.FormDiff.Modified))
	}

	if result.FormDiff.Modified[0].FieldName != "FirstName" {
		t.Errorf("Expected field name 'FirstName', got '%s'", result.FormDiff.Modified[0].FieldName)
	}

	if fmt.Sprintf("%v", result.FormDiff.Modified[0].OldValue) != "John" {
		t.Errorf("Expected old value 'John', got '%v'", result.FormDiff.Modified[0].OldValue)
	}

	if fmt.Sprintf("%v", result.FormDiff.Modified[0].NewValue) != "Jane" {
		t.Errorf("Expected new value 'Jane', got '%v'", result.FormDiff.Modified[0].NewValue)
	}
}

func TestCompareForms_FormFieldAdded(t *testing.T) {
	// Create PDF with one form field
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	fieldBuilder1 := acroform.NewFieldBuilder(builder1.Writer())
	field1 := fieldBuilder1.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field1.SetValue("John")
	builder1.FinalizePage(page1)
	acroFormNum1, _ := fieldBuilder1.Build()
	w1 := builder1.Writer()
	pagesObjNum1 := builder1.PagesObjNum()
	catalogDict1 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum1, acroFormNum1)
	catalogNum1 := w1.AddObject([]byte(catalogDict1))
	w1.SetRoot(catalogNum1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with two form fields
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	fieldBuilder2 := acroform.NewFieldBuilder(builder2.Writer())
	field2a := fieldBuilder2.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field2a.SetValue("John")
	field2b := fieldBuilder2.AddTextField("LastName", []float64{72, 680, 300, 700}, 0)
	field2b.SetValue("Doe")
	builder2.FinalizePage(page2)
	acroFormNum2, _ := fieldBuilder2.Build()
	w2 := builder2.Writer()
	pagesObjNum2 := builder2.PagesObjNum()
	catalogDict2 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum2, acroFormNum2)
	catalogNum2 := w2.AddObject([]byte(catalogDict2))
	w2.SetRoot(catalogNum2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if result.FormDiff == nil {
		t.Fatal("Expected form differences")
	}

	if len(result.FormDiff.Added) != 1 {
		t.Errorf("Expected 1 added field, got %d", len(result.FormDiff.Added))
	}

	if result.FormDiff.Added[0].FieldName != "LastName" {
		t.Errorf("Expected added field name 'LastName', got '%s'", result.FormDiff.Added[0].FieldName)
	}
}

func TestCompareForms_FormFieldRemoved(t *testing.T) {
	// Create PDF with two form fields
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	fieldBuilder1 := acroform.NewFieldBuilder(builder1.Writer())
	field1a := fieldBuilder1.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field1a.SetValue("John")
	field1b := fieldBuilder1.AddTextField("LastName", []float64{72, 680, 300, 700}, 0)
	field1b.SetValue("Doe")
	builder1.FinalizePage(page1)
	acroFormNum1, _ := fieldBuilder1.Build()
	w1 := builder1.Writer()
	pagesObjNum1 := builder1.PagesObjNum()
	catalogDict1 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum1, acroFormNum1)
	catalogNum1 := w1.AddObject([]byte(catalogDict1))
	w1.SetRoot(catalogNum1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with one form field
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	fieldBuilder2 := acroform.NewFieldBuilder(builder2.Writer())
	field2 := fieldBuilder2.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field2.SetValue("John")
	builder2.FinalizePage(page2)
	acroFormNum2, _ := fieldBuilder2.Build()
	w2 := builder2.Writer()
	pagesObjNum2 := builder2.PagesObjNum()
	catalogDict2 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum2, acroFormNum2)
	catalogNum2 := w2.AddObject([]byte(catalogDict2))
	w2.SetRoot(catalogNum2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if result.FormDiff == nil {
		t.Fatal("Expected form differences")
	}

	if len(result.FormDiff.Removed) != 1 {
		t.Errorf("Expected 1 removed field, got %d", len(result.FormDiff.Removed))
	}

	if result.FormDiff.Removed[0].FieldName != "LastName" {
		t.Errorf("Expected removed field name 'LastName', got '%s'", result.FormDiff.Removed[0].FieldName)
	}
}

func TestCompareForms_IdenticalForms(t *testing.T) {
	// Create two PDFs with identical form fields
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	fieldBuilder1 := acroform.NewFieldBuilder(builder1.Writer())
	field1 := fieldBuilder1.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field1.SetValue("John")
	builder1.FinalizePage(page1)
	acroFormNum1, _ := fieldBuilder1.Build()
	w1 := builder1.Writer()
	pagesObjNum1 := builder1.PagesObjNum()
	catalogDict1 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum1, acroFormNum1)
	catalogNum1 := w1.AddObject([]byte(catalogDict1))
	w1.SetRoot(catalogNum1)
	pdf1, _ := builder1.Bytes()

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	fieldBuilder2 := acroform.NewFieldBuilder(builder2.Writer())
	field2 := fieldBuilder2.AddTextField("FirstName", []float64{72, 700, 300, 720}, 0)
	field2.SetValue("John")
	builder2.FinalizePage(page2)
	acroFormNum2, _ := fieldBuilder2.Build()
	w2 := builder2.Writer()
	pagesObjNum2 := builder2.PagesObjNum()
	catalogDict2 := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>", pagesObjNum2, acroFormNum2)
	catalogNum2 := w2.AddObject([]byte(catalogDict2))
	w2.SetRoot(catalogNum2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	// Should have no form differences
	if result.FormDiff != nil {
		if len(result.FormDiff.Added) > 0 || len(result.FormDiff.Removed) > 0 || len(result.FormDiff.Modified) > 0 {
			t.Errorf("Expected no form differences, got added=%d, removed=%d, modified=%d",
				len(result.FormDiff.Added), len(result.FormDiff.Removed), len(result.FormDiff.Modified))
		}
	}
}
