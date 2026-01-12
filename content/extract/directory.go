package extract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/benedoc-inc/pdfer/types"
)

// DirectoryOutput represents the result of extracting a PDF to a directory
type DirectoryOutput struct {
	Path          string   `json:"path"`            // Directory path
	StructureFile string   `json:"structure_file"`  // Path to structure JSON file
	ImageFiles    []string `json:"image_files"`     // Paths to extracted image files
	HasText       bool     `json:"has_text"`        // Whether PDF contains text content
	HasImages     bool     `json:"has_images"`      // Whether PDF contains images
	HasForms      bool     `json:"has_forms"`       // Whether PDF contains forms
	IsScannedPDF  bool     `json:"is_scanned_pdf"`  // Whether this appears to be a scanned PDF
	PageCount     int      `json:"page_count"`      // Number of pages
	TextCharCount int      `json:"text_char_count"` // Total characters of text content
	ImageCount    int      `json:"image_count"`     // Number of images
}

// ExtractedContent contains all extracted PDF content with file references
type ExtractedContent struct {
	Metadata    *types.DocumentMetadata `json:"metadata,omitempty"`
	Pages       []ExtractedPage         `json:"pages"`
	Bookmarks   []types.Bookmark        `json:"bookmarks,omitempty"`
	Images      []ImageReference        `json:"images,omitempty"`
	Fonts       []types.FontInfo        `json:"fonts,omitempty"`
	PDFVersion  string                  `json:"pdf_version,omitempty"`
	IsEncrypted bool                    `json:"is_encrypted"`
	Summary     ContentSummary          `json:"summary"`
}

// ExtractedPage represents a page with references to external files
type ExtractedPage struct {
	PageNumber  int                  `json:"page_number"`
	Width       float64              `json:"width"`
	Height      float64              `json:"height"`
	Rotation    int                  `json:"rotation"`
	MediaBox    *types.Rectangle     `json:"media_box,omitempty"`
	CropBox     *types.Rectangle     `json:"crop_box,omitempty"`
	Text        []types.TextElement  `json:"text,omitempty"`
	Graphics    []types.Graphic      `json:"graphics,omitempty"`
	ImageRefs   []PageImageRef       `json:"image_refs,omitempty"`
	Annotations []types.Annotation   `json:"annotations,omitempty"`
	Resources   *types.PageResources `json:"resources,omitempty"`
}

// PageImageRef references an image on a page
type PageImageRef struct {
	ImageID   string     `json:"image_id"`            // Reference to ImageReference.ID
	X         float64    `json:"x"`                   // X position
	Y         float64    `json:"y"`                   // Y position
	Width     float64    `json:"width"`               // Display width in points
	Height    float64    `json:"height"`              // Display height in points
	Transform [6]float64 `json:"transform,omitempty"` // Transformation matrix
}

// ImageReference references an extracted image file
type ImageReference struct {
	ID         string `json:"id"`                    // Original ID (e.g., "/Im1")
	Filename   string `json:"filename"`              // Filename in output directory
	Format     string `json:"format"`                // "jpeg", "png", "tiff", etc.
	Width      int    `json:"width"`                 // Width in pixels
	Height     int    `json:"height"`                // Height in pixels
	ColorSpace string `json:"color_space,omitempty"` // Color space
	Filter     string `json:"filter,omitempty"`      // Compression filter
	Size       int    `json:"size"`                  // File size in bytes
}

// ContentSummary provides a summary of extracted content
type ContentSummary struct {
	PageCount     int  `json:"page_count"`
	TextCharCount int  `json:"text_char_count"`
	ImageCount    int  `json:"image_count"`
	HasText       bool `json:"has_text"`
	HasImages     bool `json:"has_images"`
	HasForms      bool `json:"has_forms"`
	HasBookmarks  bool `json:"has_bookmarks"`
	IsScannedPDF  bool `json:"is_scanned_pdf"`
}

// ExtractToDirectory extracts a PDF to a directory with all content
// The output directory will contain:
// - content.json: Structure, text, forms, and references
// - images/: Directory containing extracted images
func ExtractToDirectory(pdfBytes []byte, password []byte, outputDir string, verbose bool) (*DirectoryOutput, error) {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create images subdirectory
	imagesDir := filepath.Join(outputDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create images directory: %w", err)
	}

	// Extract full content
	doc, err := ExtractContent(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract content: %w", err)
	}

	// Extract all images with binary data
	allImages, err := ExtractAllImages(pdfBytes, password, verbose)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to extract images: %v\n", err)
		}
		allImages = []types.Image{}
	}

	// Build extracted content structure
	extractedContent := &ExtractedContent{
		Metadata:  doc.Metadata,
		Pages:     make([]ExtractedPage, 0, len(doc.Pages)),
		Bookmarks: doc.Bookmarks,
		Images:    make([]ImageReference, 0),
		Fonts:     doc.Fonts,
	}

	if doc.Metadata != nil {
		extractedContent.PDFVersion = doc.Metadata.PDFVersion
		extractedContent.IsEncrypted = doc.Metadata.Encrypted
	}

	// Save images to files and build references
	imageFiles := make([]string, 0)
	imageRefMap := make(map[string]ImageReference)

	for i, img := range allImages {
		if len(img.Data) == 0 {
			continue
		}

		// Determine file extension
		ext := ".bin"
		switch img.Format {
		case "jpeg":
			ext = ".jpg"
		case "png":
			ext = ".png"
		case "tiff":
			ext = ".tif"
		case "jpeg2000":
			ext = ".jp2"
		}

		// Create filename
		filename := fmt.Sprintf("image_%03d%s", i+1, ext)
		filepath := filepath.Join(imagesDir, filename)

		// Write image data
		if err := os.WriteFile(filepath, img.Data, 0644); err != nil {
			if verbose {
				fmt.Printf("Warning: failed to write image %s: %v\n", filename, err)
			}
			continue
		}

		imageRef := ImageReference{
			ID:         img.ID,
			Filename:   "images/" + filename,
			Format:     img.Format,
			Width:      img.Width,
			Height:     img.Height,
			ColorSpace: img.ColorSpace,
			Filter:     img.Filter,
			Size:       len(img.Data),
		}

		extractedContent.Images = append(extractedContent.Images, imageRef)
		imageRefMap[img.ID] = imageRef
		imageFiles = append(imageFiles, filepath)
	}

	// Convert pages and build image references
	totalTextChars := 0
	hasText := false
	hasImages := len(allImages) > 0

	for _, page := range doc.Pages {
		extractedPage := ExtractedPage{
			PageNumber:  page.PageNumber,
			Width:       page.Width,
			Height:      page.Height,
			Rotation:    page.Rotation,
			MediaBox:    page.MediaBox,
			CropBox:     page.CropBox,
			Text:        page.Text,
			Graphics:    page.Graphics,
			ImageRefs:   make([]PageImageRef, 0),
			Annotations: page.Annotations,
			Resources:   page.Resources,
		}

		// Convert image refs
		for _, imgRef := range page.Images {
			pageImgRef := PageImageRef{
				ImageID:   imgRef.ImageID,
				X:         imgRef.X,
				Y:         imgRef.Y,
				Width:     imgRef.Width,
				Height:    imgRef.Height,
				Transform: imgRef.Transform,
			}
			extractedPage.ImageRefs = append(extractedPage.ImageRefs, pageImgRef)
		}

		// Count text characters
		for _, text := range page.Text {
			totalTextChars += len(text.Text)
		}
		if len(page.Text) > 0 {
			hasText = true
		}

		extractedContent.Pages = append(extractedContent.Pages, extractedPage)
	}

	// Determine if this is a scanned PDF (images but no text)
	isScannedPDF := hasImages && !hasText && len(doc.Pages) > 0

	// Build summary
	extractedContent.Summary = ContentSummary{
		PageCount:     len(doc.Pages),
		TextCharCount: totalTextChars,
		ImageCount:    len(allImages),
		HasText:       hasText,
		HasImages:     hasImages,
		HasForms:      false, // TODO: Detect forms
		HasBookmarks:  len(doc.Bookmarks) > 0,
		IsScannedPDF:  isScannedPDF,
	}

	// Write content.json
	structureFile := filepath.Join(outputDir, "content.json")
	jsonBytes, err := json.MarshalIndent(extractedContent, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	if err := os.WriteFile(structureFile, jsonBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write content.json: %w", err)
	}

	// Build output result
	output := &DirectoryOutput{
		Path:          outputDir,
		StructureFile: structureFile,
		ImageFiles:    imageFiles,
		HasText:       hasText,
		HasImages:     hasImages,
		HasForms:      false,
		IsScannedPDF:  isScannedPDF,
		PageCount:     len(doc.Pages),
		TextCharCount: totalTextChars,
		ImageCount:    len(allImages),
	}

	return output, nil
}

// ExtractToDirectoryFromFile extracts a PDF file to a directory
func ExtractToDirectoryFromFile(pdfPath string, password []byte, outputDir string, verbose bool) (*DirectoryOutput, error) {
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF file: %w", err)
	}

	return ExtractToDirectory(pdfBytes, password, outputDir, verbose)
}
