package types

import "testing"

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a < b", 1, 2, 1},
		{"a > b", 5, 3, 3},
		{"a == b", 4, 4, 4},
		{"negative", -5, -3, -5},
		{"zero", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Min(tt.a, tt.b); got != tt.want {
				t.Errorf("Min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a < b", 1, 2, 2},
		{"a > b", 5, 3, 5},
		{"a == b", 4, 4, 4},
		{"negative", -5, -3, -3},
		{"zero", 0, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Max(tt.a, tt.b); got != tt.want {
				t.Errorf("Max(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPDFEncryption(t *testing.T) {
	enc := &PDFEncryption{
		Version:         4,
		Revision:        4,
		KeyLength:       16,
		V:               4,
		R:               4,
		P:               -3084,
		EncryptMetadata: false,
	}

	if enc.Version != 4 {
		t.Errorf("Expected Version=4, got %d", enc.Version)
	}
	if enc.KeyLength != 16 {
		t.Errorf("Expected KeyLength=16, got %d", enc.KeyLength)
	}
}

func TestPDFTrailer(t *testing.T) {
	trailer := &PDFTrailer{
		RootRef:    "/Root 204 0 R",
		EncryptRef: "/Encrypt 203 0 R",
		StartXRef:  5859135,
	}

	if trailer.StartXRef != 5859135 {
		t.Errorf("Expected StartXRef=5859135, got %d", trailer.StartXRef)
	}
	if trailer.RootRef == "" {
		t.Error("Expected RootRef to be set")
	}
}

func TestFormData(t *testing.T) {
	data := FormData{
		"Field1": "value1",
		"Field2": 123,
		"Field3": true,
	}

	if len(data) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(data))
	}

	if data["Field1"] != "value1" {
		t.Errorf("Expected Field1='value1', got %v", data["Field1"])
	}
}
