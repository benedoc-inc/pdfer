// Package writer provides PDF writing capabilities including page creation
package write

import (
	"fmt"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/resources/font"
	"github.com/benedoc-inc/pdfer/v2/types"
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
	fonts       map[string]int    // font name -> object number
	images      map[string]int    // image name -> object number
	extgstates  map[string]string // ExtGState name -> inline dict string
	annotations []int             // annotation object numbers, in addition order
	content     *ContentStream
	pageObjNum  int
	pagesObjNum int
}

// NewPageBuilder creates a new page builder
func (w *PDFWriter) NewPageBuilder(size PageSize) *PageBuilder {
	return &PageBuilder{
		writer:     w,
		size:       size,
		fonts:      make(map[string]int),
		images:     make(map[string]int),
		extgstates: make(map[string]string),
		content:    NewContentStream(),
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

// AddAnnotation adds an annotation to the page.
// The annotation object is written to the PDF immediately; the /Annots array is
// wired into the page dictionary when Build is called.
func (pb *PageBuilder) AddAnnotation(a *AnnotationBuilder) *PageBuilder {
	objNum := a.build(pb.writer)
	pb.annotations = append(pb.annotations, objNum)
	return pb
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

// addExtGState registers an ExtGState resource on the page.
// name is the resource name (e.g. "WMgs") and dict is the inline PDF dict string
// (e.g. "<</Type/ExtGState/ca 0.3000/CA 0.3000>>").
func (pb *PageBuilder) addExtGState(name, dict string) {
	pb.extgstates[name] = dict
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

	// Add ExtGState entries (e.g. transparency alpha states)
	if len(pb.extgstates) > 0 {
		resources += "/ExtGState<<"
		for name, dict := range pb.extgstates {
			resources += fmt.Sprintf("/%s %s", name, dict)
		}
		resources += ">>"
	}

	resources += ">>"

	// Annotations
	annots := ""
	if len(pb.annotations) > 0 {
		var a strings.Builder
		a.WriteString("/Annots[")
		for i, objNum := range pb.annotations {
			if i > 0 {
				a.WriteByte(' ')
			}
			fmt.Fprintf(&a, "%d 0 R", objNum)
		}
		a.WriteByte(']')
		annots = a.String()
	}

	// Create page object
	pageDict := fmt.Sprintf(`<</Type/Page/Parent %d 0 R/MediaBox[0 0 %.0f %.0f]/Contents %d 0 R/Resources%s%s>>`,
		pagesObjNum, pb.size.Width, pb.size.Height, contentObjNum, resources, annots)
	pb.pageObjNum = pb.writer.AddObject([]byte(pageDict))

	return pb.pageObjNum
}

// SimplePDFBuilder provides a high-level API for creating simple PDFs
type SimplePDFBuilder struct {
	writer         *PDFWriter
	pages          []int
	pagesObjNum    int
	catalogObjNum  int
	encryptOpts    *encrypt.EncryptOptions
	acroFormObjNum int // set by RegisterAcroForm
	pdfa           *PDFAOptions
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

// RegisterAcroForm tells Bytes() to include /AcroForm N 0 R in the catalog.
// Call this after all pages are finalized and the AcroForm object has been written.
func (b *SimplePDFBuilder) RegisterAcroForm(objNum int) {
	b.acroFormObjNum = objNum
}

// SetPassword enables AES-128 encryption (V=4, R=4) on the output PDF.
// userPassword is required to open the file; ownerPassword grants full control.
// Either may be nil for no password on that role.
// Call this before Bytes().
func (b *SimplePDFBuilder) SetPassword(userPassword, ownerPassword []byte) {
	b.encryptOpts = &encrypt.EncryptOptions{
		UserPassword:  userPassword,
		OwnerPassword: ownerPassword,
	}
}

// SetPDFAMode enables PDF/A compliance for the document. Call before Bytes().
// PDF/A requires all fonts to be embedded; using AddStandardFont (Type1
// without embedding) will produce a document that fails strict validators.
func (b *SimplePDFBuilder) SetPDFAMode(opts PDFAOptions) {
	b.pdfa = &opts
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
	catalogDict := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R", b.pagesObjNum)
	if b.acroFormObjNum != 0 {
		catalogDict += fmt.Sprintf("/AcroForm %d 0 R", b.acroFormObjNum)
	}
	if b.pdfa != nil {
		now := time.Now()
		metaObjNum, outputIntentObjNum, err := b.writer.preparePDFA(*b.pdfa, now)
		if err != nil {
			return nil, fmt.Errorf("pdf/a: %w", err)
		}
		catalogDict += fmt.Sprintf("/Metadata %d 0 R", metaObjNum)
		catalogDict += fmt.Sprintf("/OutputIntents[%d 0 R]", outputIntentObjNum)
		catalogDict += "/MarkInfo<</Marked false>>"
	}
	catalogDict += ">>"
	b.catalogObjNum = b.writer.AddObject([]byte(catalogDict))
	b.writer.SetRoot(b.catalogObjNum)

	// Apply encryption if configured.
	if b.encryptOpts != nil {
		encInfo, fileID, err := encrypt.PrepareEncryption(*b.encryptOpts)
		if err != nil {
			return nil, fmt.Errorf("preparing encryption: %w", err)
		}
		encObjNum := b.writer.AddObject(encrypt.EncryptDictString(encInfo))
		b.writer.SetEncryption(encInfo, fileID)
		b.writer.SetEncryptRef(encObjNum)
	}

	return b.writer.Bytes()
}

// SetBookmarks sets bookmarks for the document
// pageObjNums will be automatically built from the pages if nil
func (b *SimplePDFBuilder) SetBookmarks(bookmarks []types.Bookmark) error {
	// Build page object number map if not provided
	pageObjNums := make(map[int]int)
	for i, pageNum := range b.pages {
		pageObjNums[i+1] = pageNum // 1-based page numbers
	}

	_, err := b.writer.SetBookmarks(bookmarks, pageObjNums)
	return err
}

// PagesObjNum returns the pages object number
func (b *SimplePDFBuilder) PagesObjNum() int {
	return b.pagesObjNum
}

// Pages returns the list of page object numbers
func (b *SimplePDFBuilder) Pages() []int {
	return b.pages
}
