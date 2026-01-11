package compare

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GenerateReport generates a human-readable report from a comparison result
func GenerateReport(result *ComparisonResult) string {
	var report strings.Builder

	report.WriteString("PDF Comparison Report\n")
	report.WriteString(strings.Repeat("=", 50) + "\n\n")

	if result.Identical {
		report.WriteString("✅ PDFs are IDENTICAL\n")
		return report.String()
	}

	report.WriteString("❌ PDFs are DIFFERENT\n\n")
	report.WriteString(fmt.Sprintf("Total Differences: %d\n\n", result.Summary.TotalDifferences))

	// Metadata differences
	if result.MetadataDiff != nil {
		report.WriteString("Metadata Differences:\n")
		report.WriteString(strings.Repeat("-", 30) + "\n")
		if result.MetadataDiff.Title != nil {
			report.WriteString(fmt.Sprintf("  Title: %v -> %v\n", result.MetadataDiff.Title.OldValue, result.MetadataDiff.Title.NewValue))
		}
		if result.MetadataDiff.Author != nil {
			report.WriteString(fmt.Sprintf("  Author: %v -> %v\n", result.MetadataDiff.Author.OldValue, result.MetadataDiff.Author.NewValue))
		}
		if result.MetadataDiff.PageCount != nil {
			report.WriteString(fmt.Sprintf("  Page Count: %v -> %v\n", result.MetadataDiff.PageCount.OldValue, result.MetadataDiff.PageCount.NewValue))
		}
		report.WriteString("\n")
	}

	// Structure differences
	if result.StructureDiff != nil {
		report.WriteString("Structure Differences:\n")
		report.WriteString(strings.Repeat("-", 30) + "\n")
		if result.StructureDiff.PageCountDiff != nil {
			report.WriteString(fmt.Sprintf("  Page Count: %v -> %v\n", result.StructureDiff.PageCountDiff.OldValue, result.StructureDiff.PageCountDiff.NewValue))
		}
		report.WriteString("\n")
	}

	// Page differences
	if len(result.PageDiffs) > 0 {
		report.WriteString("Page Differences:\n")
		report.WriteString(strings.Repeat("-", 30) + "\n")
		for _, pd := range result.PageDiffs {
			report.WriteString(fmt.Sprintf("\nPage %d:\n", pd.PageNumber))

			// Text differences
			if pd.TextDiff != nil {
				if len(pd.TextDiff.Added) > 0 {
					report.WriteString(fmt.Sprintf("  Text Added: %d elements\n", len(pd.TextDiff.Added)))
				}
				if len(pd.TextDiff.Removed) > 0 {
					report.WriteString(fmt.Sprintf("  Text Removed: %d elements\n", len(pd.TextDiff.Removed)))
				}
				if len(pd.TextDiff.Modified) > 0 {
					report.WriteString(fmt.Sprintf("  Text Modified: %d elements\n", len(pd.TextDiff.Modified)))
				}
			}

			// Graphic differences
			if pd.GraphicDiff != nil {
				if len(pd.GraphicDiff.Added) > 0 {
					report.WriteString(fmt.Sprintf("  Graphics Added: %d elements\n", len(pd.GraphicDiff.Added)))
				}
				if len(pd.GraphicDiff.Removed) > 0 {
					report.WriteString(fmt.Sprintf("  Graphics Removed: %d elements\n", len(pd.GraphicDiff.Removed)))
				}
			}

			// Image differences
			if pd.ImageDiff != nil {
				if len(pd.ImageDiff.Added) > 0 {
					report.WriteString(fmt.Sprintf("  Images Added: %d elements\n", len(pd.ImageDiff.Added)))
				}
				if len(pd.ImageDiff.Removed) > 0 {
					report.WriteString(fmt.Sprintf("  Images Removed: %d elements\n", len(pd.ImageDiff.Removed)))
				}
			}

			// Annotation differences
			if pd.AnnotationDiff != nil {
				if len(pd.AnnotationDiff.Added) > 0 {
					report.WriteString(fmt.Sprintf("  Annotations Added: %d elements\n", len(pd.AnnotationDiff.Added)))
				}
				if len(pd.AnnotationDiff.Removed) > 0 {
					report.WriteString(fmt.Sprintf("  Annotations Removed: %d elements\n", len(pd.AnnotationDiff.Removed)))
				}
			}

			// Other differences
			for _, d := range pd.Differences {
				report.WriteString(fmt.Sprintf("  %s: %s\n", d.Type, d.Description))
			}
		}
		report.WriteString("\n")
	}

	// Form differences
	if result.FormDiff != nil {
		report.WriteString("Form Differences:\n")
		report.WriteString(strings.Repeat("-", 30) + "\n")

		if result.FormDiff.FormType != nil {
			report.WriteString(fmt.Sprintf("  Form Type: %v -> %v\n", result.FormDiff.FormType.OldValue, result.FormDiff.FormType.NewValue))
		}

		if len(result.FormDiff.Added) > 0 {
			report.WriteString(fmt.Sprintf("  Fields Added: %d\n", len(result.FormDiff.Added)))
			for _, field := range result.FormDiff.Added {
				report.WriteString(fmt.Sprintf("    - %s: %v\n", field.FieldName, field.NewValue))
			}
		}

		if len(result.FormDiff.Removed) > 0 {
			report.WriteString(fmt.Sprintf("  Fields Removed: %d\n", len(result.FormDiff.Removed)))
			for _, field := range result.FormDiff.Removed {
				report.WriteString(fmt.Sprintf("    - %s: %v\n", field.FieldName, field.OldValue))
			}
		}

		if len(result.FormDiff.Modified) > 0 {
			report.WriteString(fmt.Sprintf("  Fields Modified: %d\n", len(result.FormDiff.Modified)))
			for _, field := range result.FormDiff.Modified {
				report.WriteString(fmt.Sprintf("    - %s: %v -> %v\n", field.FieldName, field.OldValue, field.NewValue))
			}
		}

		report.WriteString("\n")
	}

	return report.String()
}

// GenerateJSONReport generates a JSON report from a comparison result
func GenerateJSONReport(result *ComparisonResult) (string, error) {
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal comparison result: %w", err)
	}
	return string(jsonBytes), nil
}
