// Package writer provides PDF writing capabilities including page creation
package writer

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/font"
)

// PageSize represents standard page dimensions in points (1 point = 1/72 inch)
type PageSize struct {
	Width  float64
	Height float64
}

// Standard page sizes
var (
	PageSizeLetter = PageSize{612, 792}  // 8.5 x 11 inches
	PageSizeA4     = PageSize{595, 842}  // 210 x 297 mm
	PageSizeLegal  = PageSize{612, 1008} // 8.5 x 14 inches
	PageSizeA3     = PageSize{842, 1191} // 297 x 420 mm
	PageSizeA5     = PageSize{420, 595}  // 148 x 210 mm
)

// PageBuilder helps build PDF pages
type PageBuilder struct {
	writer      *PDFWriter
	size        PageSize
	fonts       map[string]int // font name -> object number
	images      map[string]int // image name -> object number
	content     *ContentStream
	pageObjNum  int
	pagesObjNum int
}

// NewPageBuilder creates a new page builder
func (w *PDFWriter) NewPageBuilder(size PageSize) *PageBuilder {
	return &PageBuilder{
		writer:  w,
		size:    size,
		fonts:   make(map[string]int),
		images:  make(map[string]int),
		content: NewContentStream(),
	}
}

// Content returns the content stream for adding graphics/text
func (pb *PageBuilder) Content() *ContentStream {
	return pb.content
}

// AddStandardFont adds a standard PDF font (Helvetica, Times-Roman, etc.)
// Returns the resource name to use (e.g., "/F1")
func (pb *PageBuilder) AddStandardFont(fontName string) string {
	// Check if already added
	for name, _ := range pb.fonts {
		if name == fontName {
			return "/" + name
		}
	}

	// Create font dictionary
	resourceName := fmt.Sprintf("F%d", len(pb.fonts)+1)
	fontDict := fmt.Sprintf("<</Type/Font/Subtype/Type1/BaseFont/%s>>", fontName)
	objNum := pb.writer.AddObject([]byte(fontDict))
	pb.fonts[resourceName] = objNum

	return "/" + resourceName
}

// AddImage adds an image and returns the resource name
func (pb *PageBuilder) AddImage(info *ImageInfo) string {
	resourceName := info.Name
	if resourceName == "" {
		resourceName = fmt.Sprintf("Im%d", len(pb.images)+1)
	}
	// Remove leading / if present
	if len(resourceName) > 0 && resourceName[0] == '/' {
		resourceName = resourceName[1:]
	}
	pb.images[resourceName] = info.ObjectNum
	return "/" + resourceName
}

// AddEmbeddedFont adds an embedded TrueType/OpenType font and returns the resource name
// The font will be subset to include only the characters added via font.AddString() or font.AddRune()
func (pb *PageBuilder) AddEmbeddedFont(f *font.Font) (string, error) {
	// Create a wrapper to make PDFWriter implement font.PDFWriter interface
	wrapper := &fontWriterWrapper{w: pb.writer}

	// Create PDF objects for the font
	fontObjs, err := f.ToPDFObjects(wrapper)
	if err != nil {
		return "", fmt.Errorf("failed to create font objects: %w", err)
	}

	// Store font by resource name
	resourceName := fontObjs.ResourceName
	// Remove leading / if present
	if len(resourceName) > 0 && resourceName[0] == '/' {
		resourceName = resourceName[1:]
	}
	pb.fonts[resourceName] = fontObjs.FontDictNum

	return "/" + resourceName, nil
}

// fontWriterWrapper wraps PDFWriter to implement font.PDFWriter interface
type fontWriterWrapper struct {
	w *PDFWriter
}

func (w *fontWriterWrapper) AddObject(content []byte) int {
	return w.w.AddObject(content)
}

func (w *fontWriterWrapper) AddStreamObject(dict map[string]interface{}, data []byte, compress bool) int {
	return w.w.AddStreamObject(Dictionary(dict), data, compress)
}

func (w *fontWriterWrapper) NextObjectNumber() int {
	return w.w.nextObjNum
}

// Build finalizes the page and returns the page object number
func (pb *PageBuilder) Build(pagesObjNum int) int {
	pb.pagesObjNum = pagesObjNum

	// Create content stream object
	contentDict := Dictionary{}
	contentObjNum := pb.writer.AddStreamObject(contentDict, pb.content.Bytes(), true)

	// Build resources dictionary
	resources := "<<"

	// Add fonts
	if len(pb.fonts) > 0 {
		resources += "/Font<<"
		for name, objNum := range pb.fonts {
			resources += fmt.Sprintf("/%s %d 0 R", name, objNum)
		}
		resources += ">>"
	}

	// Add images as XObjects
	if len(pb.images) > 0 {
		resources += "/XObject<<"
		for name, objNum := range pb.images {
			resources += fmt.Sprintf("/%s %d 0 R", name, objNum)
		}
		resources += ">>"
	}

	resources += ">>"

	// Create page object
	pageDict := fmt.Sprintf(`<</Type/Page/Parent %d 0 R/MediaBox[0 0 %.0f %.0f]/Contents %d 0 R/Resources%s>>`,
		pagesObjNum, pb.size.Width, pb.size.Height, contentObjNum, resources)
	pb.pageObjNum = pb.writer.AddObject([]byte(pageDict))

	return pb.pageObjNum
}

// SimplePDFBuilder provides a high-level API for creating simple PDFs
type SimplePDFBuilder struct {
	writer        *PDFWriter
	pages         []int
	pagesObjNum   int
	catalogObjNum int
}

// NewSimplePDFBuilder creates a new simple PDF builder
func NewSimplePDFBuilder() *SimplePDFBuilder {
	return &SimplePDFBuilder{
		writer: NewPDFWriter(),
		pages:  make([]int, 0),
	}
}

// Writer returns the underlying PDF writer for advanced operations
func (b *SimplePDFBuilder) Writer() *PDFWriter {
	return b.writer
}

// AddPage adds a new page and returns a page builder
func (b *SimplePDFBuilder) AddPage(size PageSize) *PageBuilder {
	return b.writer.NewPageBuilder(size)
}

// FinalizePage adds a built page to the document
func (b *SimplePDFBuilder) FinalizePage(pb *PageBuilder) {
	// Use a placeholder for pagesObjNum - we'll fix it later
	if b.pagesObjNum == 0 {
		// Reserve object number for pages
		b.pagesObjNum = b.writer.nextObjNum
		b.writer.nextObjNum++
	}
	pageObjNum := pb.Build(b.pagesObjNum)
	b.pages = append(b.pages, pageObjNum)
}

// Bytes returns the complete PDF
func (b *SimplePDFBuilder) Bytes() ([]byte, error) {
	// Build Kids array
	kids := "["
	for _, pageNum := range b.pages {
		kids += fmt.Sprintf("%d 0 R ", pageNum)
	}
	kids += "]"

	// Create/update Pages object
	pagesDict := fmt.Sprintf("<</Type/Pages/Kids%s/Count %d>>", kids, len(b.pages))
	b.writer.SetObject(b.pagesObjNum, []byte(pagesDict))

	// Create Catalog
	b.catalogObjNum = b.writer.AddObject([]byte(fmt.Sprintf("<</Type/Catalog/Pages %d 0 R>>", b.pagesObjNum)))
	b.writer.SetRoot(b.catalogObjNum)

	return b.writer.Bytes()
}

// PagesObjNum returns the pages object number
func (b *SimplePDFBuilder) PagesObjNum() int {
	return b.pagesObjNum
}

// Pages returns the list of page object numbers
func (b *SimplePDFBuilder) Pages() []int {
	return b.pages
}
