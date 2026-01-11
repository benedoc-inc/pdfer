package types

// ContentDocument represents the complete extracted content from a PDF
// This is the top-level structure that can be serialized to JSON
type ContentDocument struct {
	Metadata    *DocumentMetadata `json:"metadata,omitempty"`
	Pages       []Page            `json:"pages"`
	Bookmarks   []Bookmark        `json:"bookmarks,omitempty"`
	Annotations []Annotation      `json:"annotations,omitempty"`
	Images      []Image           `json:"images,omitempty"`
	Fonts       []FontInfo        `json:"fonts,omitempty"`
}

// DocumentMetadata contains document-level metadata
type DocumentMetadata struct {
	Title        string                 `json:"title,omitempty"`
	Author       string                 `json:"author,omitempty"`
	Subject      string                 `json:"subject,omitempty"`
	Keywords     string                 `json:"keywords,omitempty"`
	Creator      string                 `json:"creator,omitempty"`
	Producer     string                 `json:"producer,omitempty"`
	CreationDate string                 `json:"creation_date,omitempty"`
	ModDate      string                 `json:"mod_date,omitempty"`
	PDFVersion   string                 `json:"pdf_version,omitempty"`
	PageCount    int                    `json:"page_count"`
	Encrypted    bool                   `json:"encrypted"`
	Custom       map[string]string      `json:"custom,omitempty"`
	XMP          map[string]interface{} `json:"xmp,omitempty"`
}

// Page represents a single page with all its content
type Page struct {
	PageNumber  int            `json:"page_number"`
	Width       float64        `json:"width"`    // Media box width in points
	Height      float64        `json:"height"`   // Media box height in points
	Rotation    int            `json:"rotation"` // 0, 90, 180, or 270
	MediaBox    *Rectangle     `json:"media_box,omitempty"`
	CropBox     *Rectangle     `json:"crop_box,omitempty"`
	BleedBox    *Rectangle     `json:"bleed_box,omitempty"`
	TrimBox     *Rectangle     `json:"trim_box,omitempty"`
	ArtBox      *Rectangle     `json:"art_box,omitempty"`
	Text        []TextElement  `json:"text,omitempty"`
	Graphics    []Graphic      `json:"graphics,omitempty"`
	Images      []ImageRef     `json:"images,omitempty"`
	Annotations []Annotation   `json:"annotations,omitempty"`
	Resources   *PageResources `json:"resources,omitempty"`
}

// Rectangle represents a PDF rectangle [llx lly urx ury]
type Rectangle struct {
	LowerX float64 `json:"lower_x"`
	LowerY float64 `json:"lower_y"`
	UpperX float64 `json:"upper_x"`
	UpperY float64 `json:"upper_y"`
}

// TextElement represents a piece of text with positioning and styling
type TextElement struct {
	Text        string     `json:"text"`
	X           float64    `json:"x"`                     // X position in points
	Y           float64    `json:"y"`                     // Y position in points
	Width       float64    `json:"width"`                 // Text width in points
	Height      float64    `json:"height"`                // Text height (font size) in points
	FontName    string     `json:"font_name"`             // Font resource name (e.g., "/F1")
	FontSize    float64    `json:"font_size"`             // Font size in points
	FontFamily  string     `json:"font_family,omitempty"` // Extracted font family name
	FontStyle   string     `json:"font_style,omitempty"`  // "normal", "bold", "italic", "bold-italic"
	Color       *Color     `json:"color,omitempty"`
	CharSpacing float64    `json:"char_spacing,omitempty"`
	WordSpacing float64    `json:"word_spacing,omitempty"`
	TextRise    float64    `json:"text_rise,omitempty"`
	TextMatrix  [6]float64 `json:"text_matrix,omitempty"` // Text transformation matrix
	BoundingBox *Rectangle `json:"bounding_box,omitempty"`
}

// Graphic represents a graphics element (path, shape, etc.)
type Graphic struct {
	Type        GraphicType `json:"type"` // "path", "rectangle", "circle", "line", etc.
	Path        *Path       `json:"path,omitempty"`
	FillColor   *Color      `json:"fill_color,omitempty"`
	StrokeColor *Color      `json:"stroke_color,omitempty"`
	LineWidth   float64     `json:"line_width,omitempty"`
	LineCap     int         `json:"line_cap,omitempty"`  // 0=butt, 1=round, 2=square
	LineJoin    int         `json:"line_join,omitempty"` // 0=miter, 1=round, 2=bevel
	MiterLimit  float64     `json:"miter_limit,omitempty"`
	Transform   [6]float64  `json:"transform,omitempty"` // Transformation matrix
	BoundingBox *Rectangle  `json:"bounding_box,omitempty"`
}

// GraphicType represents the type of graphic element
type GraphicType string

const (
	GraphicTypePath      GraphicType = "path"
	GraphicTypeRectangle GraphicType = "rectangle"
	GraphicTypeCircle    GraphicType = "circle"
	GraphicTypeEllipse   GraphicType = "ellipse"
	GraphicTypeLine      GraphicType = "line"
	GraphicTypePolygon   GraphicType = "polygon"
	GraphicTypePolyline  GraphicType = "polyline"
)

// Path represents a PDF path with operations
type Path struct {
	Operations []PathOperation `json:"operations"`
	Closed     bool            `json:"closed"`
}

// PathOperation represents a single path operation
type PathOperation struct {
	Type   PathOpType `json:"type"`   // "move", "line", "curve", "close", etc.
	Points []Point    `json:"points"` // Points for this operation
}

// PathOpType represents the type of path operation
type PathOpType string

const (
	PathOpMove      PathOpType = "move"
	PathOpLine      PathOpType = "line"
	PathOpCurve     PathOpType = "curve"
	PathOpClose     PathOpType = "close"
	PathOpRectangle PathOpType = "rectangle"
)

// Point represents a 2D point
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Color represents a color in various color spaces
type Color struct {
	Space ColorSpace `json:"space"`        // "rgb", "gray", "cmyk", "lab", etc.
	R     float64    `json:"r,omitempty"`  // Red or Gray
	G     float64    `json:"g,omitempty"`  // Green
	B     float64    `json:"b,omitempty"`  // Blue
	C     float64    `json:"c,omitempty"`  // Cyan
	M     float64    `json:"m,omitempty"`  // Magenta
	Y     float64    `json:"y,omitempty"`  // Yellow
	K     float64    `json:"k,omitempty"`  // Black
	L     float64    `json:"l,omitempty"`  // L* (for Lab)
	A     float64    `json:"a,omitempty"`  // a* (for Lab)
	BB    float64    `json:"bb,omitempty"` // b* (for Lab) - using BB to avoid conflict
}

// ColorSpace represents the color space
type ColorSpace string

const (
	ColorSpaceRGB   ColorSpace = "rgb"
	ColorSpaceGray  ColorSpace = "gray"
	ColorSpaceCMYK  ColorSpace = "cmyk"
	ColorSpaceLab   ColorSpace = "lab"
	ColorSpaceIndex ColorSpace = "indexed"
)

// Image represents an embedded image
type Image struct {
	ID               string                 `json:"id"`          // Resource name (e.g., "/Im1")
	Width            int                    `json:"width"`       // Width in pixels
	Height           int                    `json:"height"`      // Height in pixels
	Format           string                 `json:"format"`      // "jpeg", "png", "tiff", etc.
	ColorSpace       string                 `json:"color_space"` // "DeviceRGB", "DeviceGray", "DeviceCMYK", etc.
	BitsPerComponent int                    `json:"bits_per_component"`
	Filter           string                 `json:"filter,omitempty"`      // Compression filter (DCTDecode, FlateDecode, etc.)
	Data             []byte                 `json:"data,omitempty"`        // Image data (base64 encoded in JSON)
	DataBase64       string                 `json:"data_base64,omitempty"` // Base64 encoded image data for JSON
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ImageRef represents a reference to an image on a page
type ImageRef struct {
	ImageID   string     `json:"image_id"`            // Reference to Image.ID
	X         float64    `json:"x"`                   // X position
	Y         float64    `json:"y"`                   // Y position
	Width     float64    `json:"width"`               // Display width in points
	Height    float64    `json:"height"`              // Display height in points
	Transform [6]float64 `json:"transform,omitempty"` // Transformation matrix
}

// FontInfo represents font information
type FontInfo struct {
	ID        string `json:"id"`   // Resource name (e.g., "/F1")
	Name      string `json:"name"` // Font name
	Family    string `json:"family,omitempty"`
	Subtype   string `json:"subtype"`            // "Type1", "TrueType", "Type0", etc.
	Embedded  bool   `json:"embedded"`           // Is font embedded?
	Encoding  string `json:"encoding,omitempty"` // "WinAnsiEncoding", "MacRomanEncoding", etc.
	ToUnicode bool   `json:"to_unicode"`         // Has ToUnicode CMap?
}

// Annotation represents a PDF annotation
type Annotation struct {
	ID         string                 `json:"id"`
	Type       AnnotationType         `json:"type"`
	PageNumber int                    `json:"page_number"`
	Rect       *Rectangle             `json:"rect"`
	Contents   string                 `json:"contents,omitempty"`
	Title      string                 `json:"title,omitempty"`
	Subject    string                 `json:"subject,omitempty"`
	Color      *Color                 `json:"color,omitempty"`
	Border     *Border                `json:"border,omitempty"`
	Actions    []Action               `json:"actions,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`

	// Link-specific
	URI         string `json:"uri,omitempty"`
	Destination string `json:"destination,omitempty"`

	// Text annotation-specific
	Open bool   `json:"open,omitempty"`
	Icon string `json:"icon,omitempty"`

	// Highlight/markup-specific
	QuadPoints []Point `json:"quad_points,omitempty"`
}

// AnnotationType represents the type of annotation
type AnnotationType string

const (
	AnnotationTypeLink      AnnotationType = "link"
	AnnotationTypeText      AnnotationType = "text"
	AnnotationTypeHighlight AnnotationType = "highlight"
	AnnotationTypeUnderline AnnotationType = "underline"
	AnnotationTypeStrikeout AnnotationType = "strikeout"
	AnnotationTypeSquiggly  AnnotationType = "squiggly"
	AnnotationTypeCircle    AnnotationType = "circle"
	AnnotationTypeSquare    AnnotationType = "square"
	AnnotationTypeLine      AnnotationType = "line"
	AnnotationTypePolygon   AnnotationType = "polygon"
	AnnotationTypePolyline  AnnotationType = "polyline"
	AnnotationTypeInk       AnnotationType = "ink"
	AnnotationTypeStamp     AnnotationType = "stamp"
	AnnotationTypeCaret     AnnotationType = "caret"
	AnnotationTypeFreeText  AnnotationType = "freetext"
	AnnotationTypePopup     AnnotationType = "popup"
)

// Border represents annotation border properties
type Border struct {
	Width float64   `json:"width"`
	Style string    `json:"style,omitempty"` // "solid", "dashed", "beveled", etc.
	Dash  []float64 `json:"dash,omitempty"`
}

// Bookmark represents a bookmark/outline item
type Bookmark struct {
	Title       string     `json:"title"`
	PageNumber  int        `json:"page_number,omitempty"`
	Destination string     `json:"destination,omitempty"`
	URI         string     `json:"uri,omitempty"`
	Color       *Color     `json:"color,omitempty"`
	Style       string     `json:"style,omitempty"` // "bold", "italic", "normal"
	Children    []Bookmark `json:"children,omitempty"`
}

// PageResources represents resources used on a page
type PageResources struct {
	Fonts       map[string]FontInfo    `json:"fonts,omitempty"`
	Images      map[string]Image       `json:"images,omitempty"`
	XObjects    map[string]XObject     `json:"xobjects,omitempty"`
	ColorSpaces map[string]string      `json:"color_spaces,omitempty"`
	Patterns    map[string]interface{} `json:"patterns,omitempty"`
	Shadings    map[string]interface{} `json:"shadings,omitempty"`
}

// XObject represents an XObject (form or image)
type XObject struct {
	ID      string     `json:"id"`
	Type    string     `json:"type"` // "Form" or "Image"
	Subtype string     `json:"subtype,omitempty"`
	Width   float64    `json:"width,omitempty"`
	Height  float64    `json:"height,omitempty"`
	Matrix  [6]float64 `json:"matrix,omitempty"`
}
