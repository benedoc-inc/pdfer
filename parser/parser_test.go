package parser

import (
	"testing"
)

func TestParsePDFTrailer(t *testing.T) {
	// Sample PDF with trailer
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Root 2 0 R
>>
endobj
trailer
<<
/Root 2 0 R
/Encrypt 3 0 R
/Info 4 0 R
>>
startxref
100
%%EOF`)

	trailer, err := ParsePDFTrailer(pdfBytes)
	if err != nil {
		t.Fatalf("ParsePDFTrailer() error = %v", err)
	}

	if trailer.RootRef == "" {
		t.Error("ParsePDFTrailer() RootRef is empty")
	}
	if trailer.EncryptRef == "" {
		t.Error("ParsePDFTrailer() EncryptRef is empty")
	}
	if trailer.StartXRef != 100 {
		t.Errorf("ParsePDFTrailer() StartXRef = %d, want 100", trailer.StartXRef)
	}
}

func TestParsePDFTrailer_NoTrailer(t *testing.T) {
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Type /Catalog
>>
endobj`)

	_, err := ParsePDFTrailer(pdfBytes)
	if err == nil {
		t.Error("ParsePDFTrailer() should return error when trailer not found")
	}
}

func TestFindObjectByNumber_DirectSearch(t *testing.T) {
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Type /Catalog
>>
endobj
2 0 obj
<<
/Type /Page
>>
endobj`)

	objIndex, err := FindObjectByNumber(pdfBytes, 1, nil, false)
	if err != nil {
		t.Fatalf("FindObjectByNumber() error = %v", err)
	}

	if objIndex == -1 {
		t.Error("FindObjectByNumber() returned -1")
	}

	// Verify it found the right object
	objPattern := []byte("1 0 obj")
	found := false
	for i := objIndex; i < len(pdfBytes) && i < objIndex+20; i++ {
		if pdfBytes[i] == objPattern[0] {
			match := true
			for j := 0; j < len(objPattern) && i+j < len(pdfBytes); j++ {
				if pdfBytes[i+j] != objPattern[j] {
					match = false
					break
				}
			}
			if match {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("FindObjectByNumber() did not find correct object")
	}
}

func TestParseTraditionalXRefTable(t *testing.T) {
	// Sample traditional xref table
	pdfBytes := []byte(`xref
0 3
0000000000 65535 f
0000000015 00000 n
0000000060 00000 n
trailer
<<
/Root 1 0 R
>>
startxref
100
%%EOF`)

	objMap, err := ParseTraditionalXRefTable(pdfBytes, 0)
	if err != nil {
		t.Fatalf("ParseTraditionalXRefTable() error = %v", err)
	}

	// Object 0 is free (f), object 1 and 2 are in-use (n)
	if _, ok := objMap[0]; ok {
		t.Error("ParseTraditionalXRefTable() should not include free objects")
	}
	if offset, ok := objMap[1]; !ok || offset != 15 {
		t.Errorf("ParseTraditionalXRefTable() objMap[1] = %v, want offset 15", offset)
	}
	if offset, ok := objMap[2]; !ok || offset != 60 {
		t.Errorf("ParseTraditionalXRefTable() objMap[2] = %v, want offset 60", offset)
	}
}

func TestParseTraditionalXRefTable_Subsections(t *testing.T) {
	// Xref table with subsections
	pdfBytes := []byte(`xref
0 2
0000000000 65535 f
0000000015 00000 n
5 1
0000000060 00000 n
trailer
<<
/Root 1 0 R
>>
startxref
100
%%EOF`)

	objMap, err := ParseTraditionalXRefTable(pdfBytes, 0)
	if err != nil {
		t.Fatalf("ParseTraditionalXRefTable() error = %v", err)
	}

	// Should have objects 0 (free), 1 (n), and 5 (n)
	if _, ok := objMap[0]; ok {
		t.Error("ParseTraditionalXRefTable() should not include free objects")
	}
	if offset, ok := objMap[1]; !ok || offset != 15 {
		t.Errorf("ParseTraditionalXRefTable() objMap[1] = %v, want offset 15", offset)
	}
	if offset, ok := objMap[5]; !ok || offset != 60 {
		t.Errorf("ParseTraditionalXRefTable() objMap[5] = %v, want offset 60", offset)
	}
}

func TestGetSampleObjectNumbers(t *testing.T) {
	objMap := map[int]int64{
		1: 100,
		2: 200,
		3: 300,
		4: 400,
		5: 500,
	}

	sample := GetSampleObjectNumbers(objMap, 3)
	if len(sample) != 3 {
		t.Errorf("GetSampleObjectNumbers() length = %d, want 3", len(sample))
	}

	// Check that all returned numbers are in the map
	for _, num := range sample {
		if _, ok := objMap[num]; !ok {
			t.Errorf("GetSampleObjectNumbers() returned number %d not in map", num)
		}
	}
}

func TestParseCrossReferenceTableWithEncryption_InvalidOffset(t *testing.T) {
	pdfBytes := []byte(`%PDF-1.4`)

	_, err := ParseCrossReferenceTableWithEncryption(pdfBytes, -1, nil, false)
	if err == nil {
		t.Error("ParseCrossReferenceTableWithEncryption() should return error for invalid offset")
	}

	_, err = ParseCrossReferenceTableWithEncryption(pdfBytes, 1000, nil, false)
	if err == nil {
		t.Error("ParseCrossReferenceTableWithEncryption() should return error for offset beyond file size")
	}
}
