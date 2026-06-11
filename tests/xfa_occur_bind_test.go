package tests

import (
	"testing"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	pdftypes "github.com/benedoc-inc/pdfer/v2/types"
)

// TestPreSTAR_OccurBindMetadata verifies on the real PreSTAR form that
// repeating subforms surface normalized <occur> cardinality and that
// match="global" fields surface their <bind> declaration.
func TestPreSTAR_OccurBindMetadata(t *testing.T) {
	pdfBytes := readESTARPDF(t, "prestar.pdf")

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm(prestar.pdf): %v", err)
	}
	schema := form.Schema()
	if schema == nil {
		t.Fatal("Schema() returned nil")
	}

	byPath := map[string]*pdftypes.FormSection{}
	unbounded := 0
	var walk func(secs []pdftypes.FormSection)
	walk = func(secs []pdftypes.FormSection) {
		for i := range secs {
			byPath[secs[i].Path] = &secs[i]
			if secs[i].Occur != nil && secs[i].Occur.Max == -1 {
				unbounded++
			}
			walk(secs[i].Children)
		}
	}
	walk(schema.Sections)

	// The dominant PreSTAR pattern: <occur max="-1"> normalizes to {1,-1,1}.
	device, ok := byPath["root.DeviceDescription.Devices.Device"]
	if !ok {
		t.Fatal("repeating section root.DeviceDescription.Devices.Device not found")
	}
	if want := (pdftypes.Occur{Min: 1, Max: -1, Initial: 1}); device.Occur == nil || *device.Occur != want {
		t.Errorf("Device section Occur = %+v, want %+v", device.Occur, want)
	}

	// PreSTAR has dozens of unbounded attachment/repeat subforms.
	if unbounded < 50 {
		t.Errorf("found %d unbounded (max=-1) sections, expected at least 50", unbounded)
	}

	// The lone bounded repeat: <occur max="10"> on the Response subform.
	response, ok := byPath["root.Questions.Response"]
	if !ok {
		t.Fatal("section root.Questions.Response not found")
	}
	if want := (pdftypes.Occur{Min: 1, Max: 10, Initial: 1}); response.Occur == nil || *response.Occur != want {
		t.Errorf("Response section Occur = %+v, want %+v", response.Occur, want)
	}

	// <bind match="global"> fields (Version / Footer / FormID) surface Bind.
	globals := map[string]bool{}
	for _, q := range schema.Questions {
		if q.Bind != nil && q.Bind.Match == "global" {
			globals[q.Name] = true
		}
	}
	if !globals["Version"] {
		t.Errorf("question 'Version' missing Bind{Match:\"global\"}; globals found: %v", globals)
	}
}
