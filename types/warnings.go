package types

import (
	"fmt"
	"time"
)

// WarningLevel represents the severity of a warning
type WarningLevel string

const (
	WarningLevelInfo    WarningLevel = "info"
	WarningLevelWarning WarningLevel = "warning"
	WarningLevelError   WarningLevel = "error" // Non-fatal error that should be reported
)

// Warning represents a non-fatal issue encountered during PDF processing
type Warning struct {
	Level     WarningLevel           // Warning severity level
	Message   string                 // Human-readable warning message
	Code      string                 // Optional warning code for categorization
	Context   map[string]interface{} // Additional context (object number, field name, etc.)
	Timestamp time.Time              // When the warning was generated
}

// Error implements the error interface so warnings can be used as errors if needed
func (w *Warning) Error() string {
	if w.Code != "" {
		return fmt.Sprintf("[%s] %s: %s", w.Level, w.Code, w.Message)
	}
	return fmt.Sprintf("[%s] %s", w.Level, w.Message)
}

// WithContext adds context to the warning and returns the same warning for chaining
func (w *Warning) WithContext(key string, value interface{}) *Warning {
	if w.Context == nil {
		w.Context = make(map[string]interface{})
	}
	w.Context[key] = value
	return w
}

// NewWarning creates a new warning with the given level and message
func NewWarning(level WarningLevel, message string) *Warning {
	return &Warning{
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// NewWarningf creates a new warning with a formatted message
func NewWarningf(level WarningLevel, format string, args ...interface{}) *Warning {
	return &Warning{
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// NewWarningWithCode creates a new warning with a code
func NewWarningWithCode(level WarningLevel, code, message string) *Warning {
	return &Warning{
		Level:     level,
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// WarningCollector collects warnings during PDF processing
type WarningCollector struct {
	warnings []*Warning
	enabled  bool
}

// NewWarningCollector creates a new warning collector
func NewWarningCollector(enabled bool) *WarningCollector {
	return &WarningCollector{
		warnings: make([]*Warning, 0),
		enabled:  enabled,
	}
}

// Add adds a warning to the collector
func (wc *WarningCollector) Add(warning *Warning) {
	if wc.enabled && warning != nil {
		wc.warnings = append(wc.warnings, warning)
	}
}

// AddWarning adds a warning with the given level and message
func (wc *WarningCollector) AddWarning(level WarningLevel, message string) {
	wc.Add(NewWarning(level, message))
}

// AddWarningf adds a warning with a formatted message
func (wc *WarningCollector) AddWarningf(level WarningLevel, format string, args ...interface{}) {
	wc.Add(NewWarningf(level, format, args...))
}

// AddWarningWithCode adds a warning with a code
func (wc *WarningCollector) AddWarningWithCode(level WarningLevel, code, message string) {
	wc.Add(NewWarningWithCode(level, code, message))
}

// Warnings returns all collected warnings
func (wc *WarningCollector) Warnings() []*Warning {
	return wc.warnings
}

// Count returns the number of warnings collected
func (wc *WarningCollector) Count() int {
	return len(wc.warnings)
}

// HasWarnings returns true if any warnings have been collected
func (wc *WarningCollector) HasWarnings() bool {
	return len(wc.warnings) > 0
}

// Clear clears all warnings
func (wc *WarningCollector) Clear() {
	wc.warnings = wc.warnings[:0]
}

// FilterByLevel returns warnings filtered by level
func (wc *WarningCollector) FilterByLevel(level WarningLevel) []*Warning {
	result := make([]*Warning, 0)
	for _, w := range wc.warnings {
		if w.Level == level {
			result = append(result, w)
		}
	}
	return result
}

// GetByCode returns warnings filtered by code
func (wc *WarningCollector) GetByCode(code string) []*Warning {
	result := make([]*Warning, 0)
	for _, w := range wc.warnings {
		if w.Code == code {
			result = append(result, w)
		}
	}
	return result
}

// Enable enables warning collection
func (wc *WarningCollector) Enable() {
	wc.enabled = true
}

// Disable disables warning collection
func (wc *WarningCollector) Disable() {
	wc.enabled = false
}

// IsEnabled returns whether warning collection is enabled
func (wc *WarningCollector) IsEnabled() bool {
	return wc.enabled
}
