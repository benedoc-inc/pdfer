package types

// TextLabel classifies the semantic role of a text item
type TextLabel string

const (
	TextLabelText          TextLabel = "text"
	TextLabelSectionHeader TextLabel = "section_header"
	TextLabelTitle         TextLabel = "title"
	TextLabelCaption       TextLabel = "caption"
	TextLabelListItem      TextLabel = "list_item"
	TextLabelPageHeader    TextLabel = "page_header"
	TextLabelPageFooter    TextLabel = "page_footer"
	TextLabelFootnote      TextLabel = "footnote"
	TextLabelFormula       TextLabel = "formula"
)

// SemanticBBox is a bounding box with page number, using short JSON keys for Python compatibility
type SemanticBBox struct {
	Left   float64 `json:"l"`
	Right  float64 `json:"r"`
	Top    float64 `json:"t"`
	Bottom float64 `json:"b"`
	Page   int     `json:"page"`
}

// WordCell represents a single word with its bounding box
type WordCell struct {
	Text       string        `json:"text"`
	BBox       *SemanticBBox `json:"bbox,omitempty"`
	Confidence float64       `json:"confidence"`
	FromOCR    bool          `json:"from_ocr"`
}

// TextItem represents a semantically-labeled block of text (paragraph, heading, list item, etc.)
type TextItem struct {
	Text      string        `json:"text"`
	Label     TextLabel     `json:"label"`
	BBox      *SemanticBBox `json:"bbox,omitempty"`
	WordCells []WordCell    `json:"word_cells,omitempty"`
	Level     int           `json:"level,omitempty"` // Heading level (1=largest)
}

// TableCell represents a single cell in a detected table
type TableCell struct {
	Text     string `json:"text"`
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	RowSpan  int    `json:"row_span,omitempty"`
	ColSpan  int    `json:"col_span,omitempty"`
	IsHeader bool   `json:"is_header,omitempty"`
}

// TableItem represents a detected table with its cells
type TableItem struct {
	Cells      []TableCell   `json:"cells"`
	NumRows    int           `json:"num_rows"`
	NumCols    int           `json:"num_cols"`
	HeaderRows int           `json:"header_rows,omitempty"`
	BBox       *SemanticBBox `json:"bbox,omitempty"`
	Caption    string        `json:"caption,omitempty"`
}

// PictureItem represents a detected image/figure region
type PictureItem struct {
	BBox    *SemanticBBox `json:"bbox,omitempty"`
	Caption string        `json:"caption,omitempty"`
}

// FormulaItem represents a detected formula/equation
type FormulaItem struct {
	Text  string        `json:"text"`
	LaTeX string        `json:"latex,omitempty"`
	BBox  *SemanticBBox `json:"bbox,omitempty"`
}
