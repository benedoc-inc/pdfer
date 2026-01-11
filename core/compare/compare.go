package compare

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/forms"
	"github.com/benedoc-inc/pdfer/types"
)

// ComparisonResult represents the result of comparing two PDFs
type ComparisonResult struct {
	Identical     bool                 `json:"identical"`
	Differences   []Difference         `json:"differences"`
	Summary       ComparisonSummary    `json:"summary"`
	MetadataDiff  *MetadataDifference  `json:"metadata_diff,omitempty"`
	PageDiffs     []PageDifference     `json:"page_diffs,omitempty"`
	StructureDiff *StructureDifference `json:"structure_diff,omitempty"`
	FormDiff      *FormDiff            `json:"form_diff,omitempty"`
}

// ComparisonSummary provides a high-level summary of differences
type ComparisonSummary struct {
	TotalDifferences int  `json:"total_differences"`
	MetadataChanged  bool `json:"metadata_changed"`
	PagesChanged     bool `json:"pages_changed"`
	StructureChanged bool `json:"structure_changed"`
	ContentChanged   bool `json:"content_changed"`
}

// Difference represents a single difference between two PDFs
type Difference struct {
	Type        DifferenceType `json:"type"`     // "metadata", "page_count", "page_content", "structure", etc.
	Category    string         `json:"category"` // "added", "removed", "modified"
	Description string         `json:"description"`
	Location    string         `json:"location,omitempty"` // e.g., "Page 3", "Metadata.Title"
	OldValue    interface{}    `json:"old_value,omitempty"`
	NewValue    interface{}    `json:"new_value,omitempty"`
}

// DifferenceType represents the type of difference
type DifferenceType string

const (
	DifferenceTypeMetadata    DifferenceType = "metadata"
	DifferenceTypePageCount   DifferenceType = "page_count"
	DifferenceTypePageContent DifferenceType = "page_content"
	DifferenceTypeStructure   DifferenceType = "structure"
	DifferenceTypeText        DifferenceType = "text"
	DifferenceTypeGraphic     DifferenceType = "graphic"
	DifferenceTypeImage       DifferenceType = "image"
	DifferenceTypeAnnotation  DifferenceType = "annotation"
	DifferenceTypeBookmark    DifferenceType = "bookmark"
	DifferenceTypeForm        DifferenceType = "form"
)

// MetadataDifference represents differences in document metadata
type MetadataDifference struct {
	Title        *FieldDiff `json:"title,omitempty"`
	Author       *FieldDiff `json:"author,omitempty"`
	Subject      *FieldDiff `json:"subject,omitempty"`
	Keywords     *FieldDiff `json:"keywords,omitempty"`
	Creator      *FieldDiff `json:"creator,omitempty"`
	Producer     *FieldDiff `json:"producer,omitempty"`
	CreationDate *FieldDiff `json:"creation_date,omitempty"`
	ModDate      *FieldDiff `json:"mod_date,omitempty"`
	PDFVersion   *FieldDiff `json:"pdf_version,omitempty"`
	PageCount    *FieldDiff `json:"page_count,omitempty"`
	Encrypted    *FieldDiff `json:"encrypted,omitempty"`
}

// FieldDiff represents a difference in a single field
type FieldDiff struct {
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

// PageDifference represents differences in a specific page
type PageDifference struct {
	PageNumber     int             `json:"page_number"`
	Differences    []Difference    `json:"differences"`
	TextDiff       *TextDiff       `json:"text_diff,omitempty"`
	GraphicDiff    *GraphicDiff    `json:"graphic_diff,omitempty"`
	ImageDiff      *ImageDiff      `json:"image_diff,omitempty"`
	AnnotationDiff *AnnotationDiff `json:"annotation_diff,omitempty"`
}

// TextDiff represents differences in text content
type TextDiff struct {
	Added    []types.TextElement `json:"added,omitempty"`
	Removed  []types.TextElement `json:"removed,omitempty"`
	Modified []TextModification  `json:"modified,omitempty"`
}

// TextModification represents a modified text element
type TextModification struct {
	Old types.TextElement `json:"old"`
	New types.TextElement `json:"new"`
}

// GraphicDiff represents differences in graphics
type GraphicDiff struct {
	Added   []types.Graphic `json:"added,omitempty"`
	Removed []types.Graphic `json:"removed,omitempty"`
}

// ImageDiff represents differences in images
type ImageDiff struct {
	Added    []types.ImageRef    `json:"added,omitempty"`
	Removed  []types.ImageRef    `json:"removed,omitempty"`
	Modified []ImageModification `json:"modified,omitempty"`
	Moved    []ImageModification `json:"moved,omitempty"` // Same binary data, different position
}

// ImageModification represents an image that was modified (same ID/position, different binary data) or moved (same binary data, different position)
type ImageModification struct {
	Old      types.ImageRef `json:"old"`
	New      types.ImageRef `json:"new"`
	OldImage *types.Image   `json:"old_image,omitempty"` // Full image data for old
	NewImage *types.Image   `json:"new_image,omitempty"` // Full image data for new
}

// AnnotationDiff represents differences in annotations
type AnnotationDiff struct {
	Added   []types.Annotation `json:"added,omitempty"`
	Removed []types.Annotation `json:"removed,omitempty"`
}

// StructureDifference represents differences in PDF structure
type StructureDifference struct {
	ObjectCountDiff *FieldDiff `json:"object_count_diff,omitempty"`
	PageCountDiff   *FieldDiff `json:"page_count_diff,omitempty"`
}

// FormDiff represents differences in form fields
type FormDiff struct {
	Added    []FormFieldChange `json:"added,omitempty"`     // Fields added in new PDF
	Removed  []FormFieldChange `json:"removed,omitempty"`   // Fields removed in new PDF
	Modified []FormFieldChange `json:"modified,omitempty"`  // Fields with changed values
	FormType *FieldDiff        `json:"form_type,omitempty"` // Form type changed (AcroForm <-> XFA)
}

// FormFieldChange represents a change in a form field
type FormFieldChange struct {
	FieldName string      `json:"field_name"`
	OldValue  interface{} `json:"old_value,omitempty"`
	NewValue  interface{} `json:"new_value,omitempty"`
	FieldType string      `json:"field_type,omitempty"`
}

// TextGranularity defines the level of detail for text comparison
type TextGranularity string

const (
	GranularityElement TextGranularity = "element" // Compare entire text elements (default)
	GranularityWord    TextGranularity = "word"    // Compare word-by-word within elements
	GranularityChar    TextGranularity = "char"    // Compare character-by-character
)

// DiffSensitivity controls how sensitive the diff algorithm is to changes
type DiffSensitivity string

const (
	SensitivityStrict  DiffSensitivity = "strict"  // Report all differences, even minor ones
	SensitivityNormal  DiffSensitivity = "normal"  // Balance between detail and noise (default)
	SensitivityRelaxed DiffSensitivity = "relaxed" // Only report significant changes
)

// CompareOptions configures PDF comparison behavior
type CompareOptions struct {
	// Metadata options
	IgnoreMetadata bool // Ignore metadata differences (Producer, CreationDate, etc.)
	IgnoreProducer bool // Ignore Producer field differences
	IgnoreDates    bool // Ignore CreationDate and ModDate differences

	// Position tolerance
	TextTolerance    float64 // Position tolerance for text matching (default: 5.0 points)
	GraphicTolerance float64 // Position tolerance for graphic matching (default: 5.0 points)

	// Text comparison granularity and specificity
	TextGranularity    TextGranularity // Level of text comparison: element, word, or char (default: element)
	DiffSensitivity    DiffSensitivity // How sensitive to changes: strict, normal, or relaxed (default: normal)
	DetectMoves        bool            // Detect when text/images move (default: true)
	MoveTolerance      float64         // Position tolerance for detecting moves (default: 10x TextTolerance)
	MinChangeThreshold float64         // Minimum change percentage to report (0.0-1.0, default: 0.0 = report all)
	IgnoreWhitespace   bool            // Ignore whitespace differences in text (default: false)
	IgnoreCase         bool            // Case-insensitive text comparison (default: false)

	// Performance options
	Verbose bool // Enable verbose logging
}

// DefaultCompareOptions returns default comparison options
func DefaultCompareOptions() CompareOptions {
	return CompareOptions{
		TextTolerance:    5.0,
		GraphicTolerance: 5.0,
		IgnoreProducer:   true, // Producer often differs between tools
		IgnoreDates:      true, // Dates often differ
	}
}

// ComparePDFs compares two PDFs and returns a detailed comparison result
func ComparePDFs(pdf1Bytes, pdf2Bytes []byte, password1, password2 []byte, verbose bool) (*ComparisonResult, error) {
	return ComparePDFsWithOptions(pdf1Bytes, pdf2Bytes, password1, password2, DefaultCompareOptions())
}

// ComparePDFsWithOptions compares two PDFs with custom options
func ComparePDFsWithOptions(pdf1Bytes, pdf2Bytes []byte, password1, password2 []byte, opts CompareOptions) (*ComparisonResult, error) {
	// Extract content from both PDFs
	doc1, err := extract.ExtractContent(pdf1Bytes, password1, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract content from first PDF: %w", err)
	}

	doc2, err := extract.ExtractContent(pdf2Bytes, password2, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract content from second PDF: %w", err)
	}

	result := &ComparisonResult{
		Differences: []Difference{},
		Summary:     ComparisonSummary{},
	}

	// Compare metadata (with options)
	metadataDiff := compareMetadata(doc1.Metadata, doc2.Metadata, opts)
	if metadataDiff != nil {
		result.MetadataDiff = metadataDiff
		result.Summary.MetadataChanged = true
		result.Summary.TotalDifferences++
	}

	// Compare structure (page count, etc.)
	structureDiff := compareStructure(doc1, doc2)
	if structureDiff != nil {
		result.StructureDiff = structureDiff
		result.Summary.StructureChanged = true
		result.Summary.TotalDifferences++
	}

	// Compare pages
	pageDiffs := comparePages(doc1.Pages, doc2.Pages, opts, pdf1Bytes, pdf2Bytes)
	if len(pageDiffs) > 0 {
		result.PageDiffs = pageDiffs
		result.Summary.PagesChanged = true
		result.Summary.ContentChanged = true
		for _, pd := range pageDiffs {
			result.Summary.TotalDifferences += len(pd.Differences)
		}
	}

	// Compare bookmarks
	bookmarkDiff := compareBookmarks(doc1.Bookmarks, doc2.Bookmarks)
	if bookmarkDiff != nil {
		result.Differences = append(result.Differences, *bookmarkDiff)
		result.Summary.TotalDifferences++
	}

	// Compare forms
	formDiff := compareForms(pdf1Bytes, pdf2Bytes, password1, password2, opts.Verbose)
	if formDiff != nil && (len(formDiff.Added) > 0 || len(formDiff.Removed) > 0 || len(formDiff.Modified) > 0 || formDiff.FormType != nil) {
		result.FormDiff = formDiff
		result.Differences = append(result.Differences, Difference{
			Type:        DifferenceTypeForm,
			Category:    "modified",
			Description: fmt.Sprintf("Form fields changed: %d added, %d removed, %d modified", len(formDiff.Added), len(formDiff.Removed), len(formDiff.Modified)),
		})
		result.Summary.TotalDifferences++
		result.Summary.ContentChanged = true
	}

	// Determine if identical
	result.Identical = result.Summary.TotalDifferences == 0

	return result, nil
}

// compareMetadata compares document metadata
func compareMetadata(m1, m2 *types.DocumentMetadata, opts CompareOptions) *MetadataDifference {
	if opts.IgnoreMetadata {
		return nil
	}

	if m1 == nil && m2 == nil {
		return nil
	}
	if m1 == nil || m2 == nil {
		// One is nil, other is not - significant difference
		return &MetadataDifference{}
	}

	diff := &MetadataDifference{}
	hasDiff := false

	if m1.Title != m2.Title {
		diff.Title = &FieldDiff{OldValue: m1.Title, NewValue: m2.Title}
		hasDiff = true
	}
	if m1.Author != m2.Author {
		diff.Author = &FieldDiff{OldValue: m1.Author, NewValue: m2.Author}
		hasDiff = true
	}
	if m1.Subject != m2.Subject {
		diff.Subject = &FieldDiff{OldValue: m1.Subject, NewValue: m2.Subject}
		hasDiff = true
	}
	if m1.Keywords != m2.Keywords {
		diff.Keywords = &FieldDiff{OldValue: m1.Keywords, NewValue: m2.Keywords}
		hasDiff = true
	}
	if m1.Creator != m2.Creator {
		diff.Creator = &FieldDiff{OldValue: m1.Creator, NewValue: m2.Creator}
		hasDiff = true
	}
	if !opts.IgnoreProducer && m1.Producer != m2.Producer {
		diff.Producer = &FieldDiff{OldValue: m1.Producer, NewValue: m2.Producer}
		hasDiff = true
	}
	if !opts.IgnoreDates {
		if m1.CreationDate != m2.CreationDate {
			diff.CreationDate = &FieldDiff{OldValue: m1.CreationDate, NewValue: m2.CreationDate}
			hasDiff = true
		}
		if m1.ModDate != m2.ModDate {
			diff.ModDate = &FieldDiff{OldValue: m1.ModDate, NewValue: m2.ModDate}
			hasDiff = true
		}
	}
	if m1.PDFVersion != m2.PDFVersion {
		diff.PDFVersion = &FieldDiff{OldValue: m1.PDFVersion, NewValue: m2.PDFVersion}
		hasDiff = true
	}
	if m1.PageCount != m2.PageCount {
		diff.PageCount = &FieldDiff{OldValue: m1.PageCount, NewValue: m2.PageCount}
		hasDiff = true
	}
	if m1.Encrypted != m2.Encrypted {
		diff.Encrypted = &FieldDiff{OldValue: m1.Encrypted, NewValue: m2.Encrypted}
		hasDiff = true
	}

	if !hasDiff {
		return nil
	}
	return diff
}

// compareStructure compares PDF structure
func compareStructure(doc1, doc2 *types.ContentDocument) *StructureDifference {
	diff := &StructureDifference{}
	hasDiff := false

	// Compare page counts
	if len(doc1.Pages) != len(doc2.Pages) {
		diff.PageCountDiff = &FieldDiff{
			OldValue: len(doc1.Pages),
			NewValue: len(doc2.Pages),
		}
		hasDiff = true
	}

	// Note: Object count would require parsing both PDFs, which is more expensive
	// For now, we skip it or could add it as an optional comparison

	if !hasDiff {
		return nil
	}
	return diff
}

// comparePages compares pages between two documents
func comparePages(pages1, pages2 []types.Page, opts CompareOptions, pdf1Bytes, pdf2Bytes []byte) []PageDifference {
	var diffs []PageDifference

	// Compare up to the minimum page count
	minPages := len(pages1)
	if len(pages2) < minPages {
		minPages = len(pages2)
	}

	for i := 0; i < minPages; i++ {
		pageDiff := compareSinglePage(pages1[i], pages2[i], i+1, opts, pdf1Bytes, pdf2Bytes)
		if pageDiff != nil && (len(pageDiff.Differences) > 0 || pageDiff.TextDiff != nil || pageDiff.GraphicDiff != nil || pageDiff.ImageDiff != nil || pageDiff.AnnotationDiff != nil) {
			diffs = append(diffs, *pageDiff)
		}
	}

	// Handle extra pages in either document
	if len(pages1) > len(pages2) {
		for i := len(pages2); i < len(pages1); i++ {
			diffs = append(diffs, PageDifference{
				PageNumber: i + 1,
				Differences: []Difference{{
					Type:        DifferenceTypePageContent,
					Category:    "removed",
					Description: fmt.Sprintf("Page %d removed in second PDF", i+1),
					Location:    fmt.Sprintf("Page %d", i+1),
				}},
			})
		}
	} else if len(pages2) > len(pages1) {
		for i := len(pages1); i < len(pages2); i++ {
			diffs = append(diffs, PageDifference{
				PageNumber: i + 1,
				Differences: []Difference{{
					Type:        DifferenceTypePageContent,
					Category:    "added",
					Description: fmt.Sprintf("Page %d added in second PDF", i+1),
					Location:    fmt.Sprintf("Page %d", i+1),
				}},
			})
		}
	}

	return diffs
}

// compareSinglePage compares a single page between two documents
func compareSinglePage(page1, page2 types.Page, pageNum int, opts CompareOptions, pdf1Bytes, pdf2Bytes []byte) *PageDifference {
	diff := &PageDifference{
		PageNumber:  pageNum,
		Differences: []Difference{},
	}

	// Compare page dimensions
	if page1.Width != page2.Width || page1.Height != page2.Height {
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypePageContent,
			Category:    "modified",
			Description: fmt.Sprintf("Page dimensions changed: %.2fx%.2f -> %.2fx%.2f", page1.Width, page1.Height, page2.Width, page2.Height),
			Location:    fmt.Sprintf("Page %d", pageNum),
			OldValue:    fmt.Sprintf("%.2fx%.2f", page1.Width, page1.Height),
			NewValue:    fmt.Sprintf("%.2fx%.2f", page2.Width, page2.Height),
		})
	}

	// Compare rotation
	if page1.Rotation != page2.Rotation {
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypePageContent,
			Category:    "modified",
			Description: fmt.Sprintf("Page rotation changed: %d° -> %d°", page1.Rotation, page2.Rotation),
			Location:    fmt.Sprintf("Page %d", pageNum),
			OldValue:    page1.Rotation,
			NewValue:    page2.Rotation,
		})
	}

	// Compare text
	textDiff := compareText(page1.Text, page2.Text, opts)
	if textDiff != nil && (len(textDiff.Added) > 0 || len(textDiff.Removed) > 0 || len(textDiff.Modified) > 0) {
		diff.TextDiff = textDiff
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypeText,
			Category:    "modified",
			Description: fmt.Sprintf("Text content changed: %d added, %d removed, %d modified", len(textDiff.Added), len(textDiff.Removed), len(textDiff.Modified)),
			Location:    fmt.Sprintf("Page %d", pageNum),
		})
	}

	// Compare graphics
	graphicDiff := compareGraphics(page1.Graphics, page2.Graphics)
	if graphicDiff != nil && (len(graphicDiff.Added) > 0 || len(graphicDiff.Removed) > 0) {
		diff.GraphicDiff = graphicDiff
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypeGraphic,
			Category:    "modified",
			Description: fmt.Sprintf("Graphics changed: %d added, %d removed", len(graphicDiff.Added), len(graphicDiff.Removed)),
			Location:    fmt.Sprintf("Page %d", pageNum),
		})
	}

	// Compare images (with binary data comparison)
	imageDiff := compareImagesWithBinary(page1.Images, page2.Images, page1.Resources, page2.Resources, pdf1Bytes, pdf2Bytes)
	if imageDiff != nil && (len(imageDiff.Added) > 0 || len(imageDiff.Removed) > 0 || len(imageDiff.Modified) > 0 || len(imageDiff.Moved) > 0) {
		diff.ImageDiff = imageDiff
		desc := fmt.Sprintf("Images changed: %d added, %d removed, %d modified", len(imageDiff.Added), len(imageDiff.Removed), len(imageDiff.Modified))
		if len(imageDiff.Moved) > 0 {
			desc += fmt.Sprintf(", %d moved", len(imageDiff.Moved))
		}
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypeImage,
			Category:    "modified",
			Description: desc,
			Location:    fmt.Sprintf("Page %d", pageNum),
		})
	} else if len(page1.Images) != len(page2.Images) {
		// Different number of images even if diff is nil (might be due to extraction issues)
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypeImage,
			Category:    "modified",
			Description: fmt.Sprintf("Image count changed: %d -> %d", len(page1.Images), len(page2.Images)),
			Location:    fmt.Sprintf("Page %d", pageNum),
			OldValue:    len(page1.Images),
			NewValue:    len(page2.Images),
		})
	}

	// Compare annotations
	annotationDiff := compareAnnotations(page1.Annotations, page2.Annotations)
	if annotationDiff != nil && (len(annotationDiff.Added) > 0 || len(annotationDiff.Removed) > 0) {
		diff.AnnotationDiff = annotationDiff
		diff.Differences = append(diff.Differences, Difference{
			Type:        DifferenceTypeAnnotation,
			Category:    "modified",
			Description: fmt.Sprintf("Annotations changed: %d added, %d removed", len(annotationDiff.Added), len(annotationDiff.Removed)),
			Location:    fmt.Sprintf("Page %d", pageNum),
		})
	}

	if len(diff.Differences) == 0 && diff.TextDiff == nil && diff.GraphicDiff == nil && diff.ImageDiff == nil && diff.AnnotationDiff == nil {
		return nil
	}

	return diff
}

// compareText compares text elements between two pages
// Implementation moved to text_diff.go for better organization
// This is a placeholder - the actual implementation is in text_diff.go

// textElementsEqual compares two text elements for equality
func textElementsEqual(t1, t2 types.TextElement) bool {
	return t1.Text == t2.Text &&
		positionsMatch(t1.X, t1.Y, t2.X, t2.Y, 0.1) &&
		t1.FontName == t2.FontName &&
		abs(t1.FontSize-t2.FontSize) < 0.1
}

// positionsMatch checks if two positions match within tolerance
func positionsMatch(x1, y1, x2, y2, tolerance float64) bool {
	return abs(x1-x2) < tolerance && abs(y1-y2) < tolerance
}

// abs returns absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// compareGraphics compares graphics between two pages
func compareGraphics(g1, g2 []types.Graphic) *GraphicDiff {
	diff := &GraphicDiff{
		Added:   []types.Graphic{},
		Removed: []types.Graphic{},
	}

	// Simple comparison: compare by type and approximate position
	// More sophisticated matching could use shape matching
	g1Map := make(map[string]types.Graphic)
	for _, g := range g1 {
		key := graphicKey(g)
		g1Map[key] = g
	}

	for _, g := range g2 {
		key := graphicKey(g)
		if _, exists := g1Map[key]; !exists {
			diff.Added = append(diff.Added, g)
		} else {
			delete(g1Map, key)
		}
	}

	// Remaining in g1Map are removed
	for _, g := range g1Map {
		diff.Removed = append(diff.Removed, g)
	}

	if len(diff.Added) == 0 && len(diff.Removed) == 0 {
		return nil
	}

	return diff
}

// graphicKey generates a key for a graphic element for comparison
func graphicKey(g types.Graphic) string {
	// Use type and bounding box for key
	if g.BoundingBox != nil {
		return fmt.Sprintf("%s:%.1f:%.1f:%.1f:%.1f", g.Type, g.BoundingBox.LowerX, g.BoundingBox.LowerY, g.BoundingBox.UpperX, g.BoundingBox.UpperY)
	}
	return fmt.Sprintf("%s", g.Type)
}

// compareImagesWithBinary compares images between two pages, including binary data
func compareImagesWithBinary(img1, img2 []types.ImageRef, resources1, resources2 *types.PageResources, pdf1Bytes, pdf2Bytes []byte) *ImageDiff {
	diff := &ImageDiff{
		Added:    []types.ImageRef{},
		Removed:  []types.ImageRef{},
		Modified: []ImageModification{},
		Moved:    []ImageModification{},
	}

	// Extract full image data for all images
	images1Map := make(map[string]*types.Image) // Map ImageID -> full Image data
	images2Map := make(map[string]*types.Image)

	// Extract images from PDF1
	if resources1 != nil {
		for name, img := range resources1.Images {
			imageID := "/" + name
			// Get full image data with binary
			fullImg, err := getImageWithBinary(imageID, pdf1Bytes, &img)
			if err == nil {
				images1Map[imageID] = fullImg
			} else {
				// Fall back to metadata-only image
				images1Map[imageID] = &img
			}
		}
	}

	// Extract images from PDF2
	if resources2 != nil {
		for name, img := range resources2.Images {
			imageID := "/" + name
			// Get full image data with binary
			fullImg, err := getImageWithBinary(imageID, pdf2Bytes, &img)
			if err == nil {
				images2Map[imageID] = fullImg
			} else {
				// Fall back to metadata-only image
				images2Map[imageID] = &img
			}
		}
	}

	// Compare by image ID and position (with tolerance for position)
	img1Matched := make(map[int]bool)
	img2Matched := make(map[int]bool)

	// First pass: exact matches (same ID, position, and binary data)
	for i1, imgRef1 := range img1 {
		if img1Matched[i1] {
			continue
		}
		for i2, imgRef2 := range img2 {
			if img2Matched[i2] {
				continue
			}
			if imgRef1.ImageID == imgRef2.ImageID && positionsMatch(imgRef1.X, imgRef1.Y, imgRef2.X, imgRef2.Y, 1.0) {
				// Same ID and position - check binary data
				img1Data := images1Map[imgRef1.ImageID]
				img2Data := images2Map[imgRef2.ImageID]
				if img1Data != nil && img2Data != nil {
					if imagesEqual(img1Data, img2Data) {
						// Exact match including binary
						img1Matched[i1] = true
						img2Matched[i2] = true
						break
					} else {
						// Same ID/position but different binary - mark as modified
						diff.Modified = append(diff.Modified, ImageModification{
							Old:      imgRef1,
							New:      imgRef2,
							OldImage: img1Data,
							NewImage: img2Data,
						})
						img1Matched[i1] = true
						img2Matched[i2] = true
						break
					}
				} else {
					// Can't compare binary (missing data) - treat as match for now
					img1Matched[i1] = true
					img2Matched[i2] = true
					break
				}
			}
		}
	}

	// Second pass: find moved images (same binary data, different position)
	// This handles cases where the same image is moved to a different position
	for i1, imgRef1 := range img1 {
		if img1Matched[i1] {
			continue
		}
		img1Data := images1Map[imgRef1.ImageID]
		if img1Data == nil || len(img1Data.Data) == 0 {
			continue // Can't match by binary without data
		}

		for i2, imgRef2 := range img2 {
			if img2Matched[i2] {
				continue
			}
			img2Data := images2Map[imgRef2.ImageID]
			if img2Data == nil || len(img2Data.Data) == 0 {
				continue
			}

			// Check if binary data matches (ignoring position)
			if imagesEqual(img1Data, img2Data) {
				// Same binary data but different position - it's a move
				diff.Moved = append(diff.Moved, ImageModification{
					Old:      imgRef1,
					New:      imgRef2,
					OldImage: img1Data,
					NewImage: img2Data,
				})
				img1Matched[i1] = true
				img2Matched[i2] = true
				break
			}
		}
	}

	// Find added (in img2 but not matched)
	for i2, img2 := range img2 {
		if !img2Matched[i2] {
			diff.Added = append(diff.Added, img2)
		}
	}

	// Remaining in img1 are removed
	for i1, img1 := range img1 {
		if !img1Matched[i1] {
			diff.Removed = append(diff.Removed, img1)
		}
	}

	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Modified) == 0 && len(diff.Moved) == 0 {
		return nil
	}

	return diff
}

// getImageWithBinary extracts full image data with binary from PDF
func getImageWithBinary(imageID string, pdfBytes []byte, metadataImg *types.Image) (*types.Image, error) {
	// If metadata image already has data, use it
	if metadataImg != nil && len(metadataImg.Data) > 0 {
		return metadataImg, nil
	}

	// Otherwise, extract all images and find the one we need
	allImages, err := extract.ExtractAllImages(pdfBytes, nil, false)
	if err != nil {
		return nil, err
	}

	// Find image by ID
	for _, img := range allImages {
		if img.ID == imageID {
			return &img, nil
		}
	}

	// Not found - return metadata-only image
	if metadataImg != nil {
		return metadataImg, nil
	}
	return nil, fmt.Errorf("image %s not found", imageID)
}

// imagesEqual compares two images for binary equality
func imagesEqual(img1, img2 *types.Image) bool {
	if img1 == nil || img2 == nil {
		return img1 == img2
	}

	// Compare metadata first
	if img1.Width != img2.Width || img1.Height != img2.Height {
		return false
	}
	if img1.Format != img2.Format {
		return false
	}
	if img1.ColorSpace != img2.ColorSpace {
		return false
	}

	// Compare binary data
	if len(img1.Data) == 0 && len(img2.Data) == 0 {
		// Both have no data - compare by hash if available, or consider equal if metadata matches
		return true
	}
	if len(img1.Data) != len(img2.Data) {
		return false
	}

	// Use byte comparison for exact match
	return bytes.Equal(img1.Data, img2.Data)
}

// compareAnnotations compares annotations between two pages
func compareAnnotations(a1, a2 []types.Annotation) *AnnotationDiff {
	diff := &AnnotationDiff{
		Added:   []types.Annotation{},
		Removed: []types.Annotation{},
	}

	// Compare by type and position
	a1Map := make(map[string]types.Annotation)
	for _, a := range a1 {
		key := annotationKey(a)
		a1Map[key] = a
	}

	for _, a := range a2 {
		key := annotationKey(a)
		if _, exists := a1Map[key]; !exists {
			diff.Added = append(diff.Added, a)
		} else {
			delete(a1Map, key)
		}
	}

	// Remaining in a1Map are removed
	for _, a := range a1Map {
		diff.Removed = append(diff.Removed, a)
	}

	if len(diff.Added) == 0 && len(diff.Removed) == 0 {
		return nil
	}

	return diff
}

// annotationKey generates a key for an annotation for comparison
func annotationKey(a types.Annotation) string {
	if a.Rect != nil {
		return fmt.Sprintf("%s:%.1f:%.1f:%.1f:%.1f", a.Type, a.Rect.LowerX, a.Rect.LowerY, a.Rect.UpperX, a.Rect.UpperY)
	}
	return fmt.Sprintf("%s", a.Type)
}

// compareBookmarks compares bookmarks between two documents
func compareBookmarks(b1, b2 []types.Bookmark) *Difference {
	if len(b1) != len(b2) {
		return &Difference{
			Type:        DifferenceTypeBookmark,
			Category:    "modified",
			Description: fmt.Sprintf("Bookmark count changed: %d -> %d", len(b1), len(b2)),
			OldValue:    len(b1),
			NewValue:    len(b2),
		}
	}

	// Simple comparison - could be more sophisticated
	if !reflect.DeepEqual(b1, b2) {
		return &Difference{
			Type:        DifferenceTypeBookmark,
			Category:    "modified",
			Description: "Bookmark structure changed",
		}
	}

	return nil
}

// compareForms compares form fields between two PDFs
func compareForms(pdf1Bytes, pdf2Bytes []byte, password1, password2 []byte, verbose bool) *FormDiff {
	diff := &FormDiff{
		Added:    []FormFieldChange{},
		Removed:  []FormFieldChange{},
		Modified: []FormFieldChange{},
	}

	// Extract forms from both PDFs
	form1, err1 := forms.Extract(pdf1Bytes, password1, verbose)
	form2, err2 := forms.Extract(pdf2Bytes, password2, verbose)

	// If neither PDF has forms, no differences
	if err1 != nil && err2 != nil {
		return nil
	}

	// If one has a form and the other doesn't, that's a difference
	if err1 != nil && err2 == nil {
		// PDF1 has no form, PDF2 has form - all fields in PDF2 are "added"
		values2 := form2.GetValues()
		for name, value := range values2 {
			diff.Added = append(diff.Added, FormFieldChange{
				FieldName: name,
				NewValue:  value,
			})
		}
		return diff
	}

	if err1 == nil && err2 != nil {
		// PDF1 has form, PDF2 has no form - all fields in PDF1 are "removed"
		values1 := form1.GetValues()
		for name, value := range values1 {
			diff.Removed = append(diff.Removed, FormFieldChange{
				FieldName: name,
				OldValue:  value,
			})
		}
		return diff
	}

	// Both have forms - compare form types
	if form1.Type() != form2.Type() {
		diff.FormType = &FieldDiff{
			OldValue: string(form1.Type()),
			NewValue: string(form2.Type()),
		}
		// If form types differ, treat all fields as changed
		values1 := form1.GetValues()
		values2 := form2.GetValues()
		for name, value := range values1 {
			diff.Removed = append(diff.Removed, FormFieldChange{
				FieldName: name,
				OldValue:  value,
			})
		}
		for name, value := range values2 {
			diff.Added = append(diff.Added, FormFieldChange{
				FieldName: name,
				NewValue:  value,
			})
		}
		return diff
	}

	// Same form type - compare field values
	values1 := form1.GetValues()
	values2 := form2.GetValues()

	// Find added, removed, and modified fields
	// Fields in PDF2 but not in PDF1
	for name, value := range values2 {
		if _, exists := values1[name]; !exists {
			diff.Added = append(diff.Added, FormFieldChange{
				FieldName: name,
				NewValue:  value,
			})
		}
	}

	// Fields in PDF1 but not in PDF2
	for name, value := range values1 {
		if _, exists := values2[name]; !exists {
			diff.Removed = append(diff.Removed, FormFieldChange{
				FieldName: name,
				OldValue:  value,
			})
		}
	}

	// Fields in both - check if values changed
	for name, value1 := range values1 {
		if value2, exists := values2[name]; exists {
			if !valuesEqual(value1, value2) {
				diff.Modified = append(diff.Modified, FormFieldChange{
					FieldName: name,
					OldValue:  value1,
					NewValue:  value2,
				})
			}
		}
	}

	// Return nil if no differences
	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Modified) == 0 && diff.FormType == nil {
		return nil
	}

	return diff
}

// valuesEqual compares two form field values for equality
func valuesEqual(v1, v2 interface{}) bool {
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}

	// Convert to strings for comparison (handles different types)
	str1 := fmt.Sprintf("%v", v1)
	str2 := fmt.Sprintf("%v", v2)
	return str1 == str2
}
