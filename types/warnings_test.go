package types

import (
	"testing"
	"time"
)

func TestWarning_Error(t *testing.T) {
	tests := []struct {
		name     string
		warning  *Warning
		expected string
	}{
		{
			name:     "simple warning",
			warning:  NewWarning(WarningLevelWarning, "test warning"),
			expected: "[warning] test warning",
		},
		{
			name:     "warning with code",
			warning:  NewWarningWithCode(WarningLevelError, "MISSING_FONT", "font not found"),
			expected: "[error] MISSING_FONT: font not found",
		},
		{
			name:     "formatted warning",
			warning:  NewWarningf(WarningLevelInfo, "object %d not found", 42),
			expected: "[info] object 42 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.warning.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWarning_WithContext(t *testing.T) {
	warning := NewWarning(WarningLevelWarning, "test warning").
		WithContext("objectNum", 42).
		WithContext("generation", 0)

	if warning.Context["objectNum"] != 42 {
		t.Errorf("Context[objectNum] = %v, want 42", warning.Context["objectNum"])
	}
	if warning.Context["generation"] != 0 {
		t.Errorf("Context[generation] = %v, want 0", warning.Context["generation"])
	}
}

func TestWarningCollector_Add(t *testing.T) {
	wc := NewWarningCollector(true)

	warning := NewWarning(WarningLevelWarning, "test warning")
	wc.Add(warning)

	if wc.Count() != 1 {
		t.Errorf("Count() = %d, want 1", wc.Count())
	}

	if !wc.HasWarnings() {
		t.Error("HasWarnings() = false, want true")
	}

	warnings := wc.Warnings()
	if len(warnings) != 1 || warnings[0] != warning {
		t.Error("Warnings() did not return the added warning")
	}
}

func TestWarningCollector_Disabled(t *testing.T) {
	wc := NewWarningCollector(false)

	warning := NewWarning(WarningLevelWarning, "test warning")
	wc.Add(warning)

	if wc.Count() != 0 {
		t.Errorf("Count() = %d, want 0 when disabled", wc.Count())
	}

	if wc.HasWarnings() {
		t.Error("HasWarnings() = true, want false when disabled")
	}
}

func TestWarningCollector_AddMethods(t *testing.T) {
	wc := NewWarningCollector(true)

	wc.AddWarning(WarningLevelInfo, "info message")
	wc.AddWarningf(WarningLevelWarning, "warning %s", "message")
	wc.AddWarningWithCode(WarningLevelError, "TEST_CODE", "error message")

	if wc.Count() != 3 {
		t.Errorf("Count() = %d, want 3", wc.Count())
	}

	warnings := wc.Warnings()
	if warnings[0].Level != WarningLevelInfo || warnings[0].Message != "info message" {
		t.Error("First warning incorrect")
	}
	if warnings[1].Level != WarningLevelWarning || warnings[1].Message != "warning message" {
		t.Error("Second warning incorrect")
	}
	if warnings[2].Level != WarningLevelError || warnings[2].Code != "TEST_CODE" {
		t.Error("Third warning incorrect")
	}
}

func TestWarningCollector_FilterByLevel(t *testing.T) {
	wc := NewWarningCollector(true)

	wc.AddWarning(WarningLevelInfo, "info 1")
	wc.AddWarning(WarningLevelWarning, "warning 1")
	wc.AddWarning(WarningLevelInfo, "info 2")
	wc.AddWarning(WarningLevelError, "error 1")

	infoWarnings := wc.FilterByLevel(WarningLevelInfo)
	if len(infoWarnings) != 2 {
		t.Errorf("FilterByLevel(Info) = %d, want 2", len(infoWarnings))
	}

	warningWarnings := wc.FilterByLevel(WarningLevelWarning)
	if len(warningWarnings) != 1 {
		t.Errorf("FilterByLevel(Warning) = %d, want 1", len(warningWarnings))
	}
}

func TestWarningCollector_GetByCode(t *testing.T) {
	wc := NewWarningCollector(true)

	wc.AddWarningWithCode(WarningLevelWarning, "MISSING_FONT", "font 1")
	wc.AddWarning(WarningLevelWarning, "no code")
	wc.AddWarningWithCode(WarningLevelError, "MISSING_FONT", "font 2")

	fontWarnings := wc.GetByCode("MISSING_FONT")
	if len(fontWarnings) != 2 {
		t.Errorf("GetByCode(MISSING_FONT) = %d, want 2", len(fontWarnings))
	}
}

func TestWarningCollector_Clear(t *testing.T) {
	wc := NewWarningCollector(true)

	wc.AddWarning(WarningLevelWarning, "test")
	if wc.Count() != 1 {
		t.Error("Expected 1 warning before clear")
	}

	wc.Clear()
	if wc.Count() != 0 {
		t.Error("Expected 0 warnings after clear")
	}
}

func TestWarningCollector_EnableDisable(t *testing.T) {
	wc := NewWarningCollector(true)

	if !wc.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}

	wc.Disable()
	if wc.IsEnabled() {
		t.Error("IsEnabled() = true after Disable(), want false")
	}

	wc.AddWarning(WarningLevelWarning, "test")
	if wc.Count() != 0 {
		t.Error("Expected 0 warnings when disabled")
	}

	wc.Enable()
	if !wc.IsEnabled() {
		t.Error("IsEnabled() = false after Enable(), want true")
	}

	wc.AddWarning(WarningLevelWarning, "test")
	if wc.Count() != 1 {
		t.Error("Expected 1 warning after enabling")
	}
}

func TestWarning_Timestamp(t *testing.T) {
	before := time.Now()
	warning := NewWarning(WarningLevelWarning, "test")
	after := time.Now()

	if warning.Timestamp.Before(before) || warning.Timestamp.After(after) {
		t.Error("Warning timestamp not set correctly")
	}
}
