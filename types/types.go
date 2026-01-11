package types

// PDFEncryption represents PDF encryption parameters
type PDFEncryption struct {
	Version         int
	Revision        int
	KeyLength       int
	Filter          string
	SubFilter       string
	V               int
	R               int
	O               []byte // Owner password hash
	U               []byte // User password hash
	P               int32  // Permissions
	EncryptMetadata bool
	EncryptKey      []byte // Master encryption key
}

// PDFTrailer represents PDF trailer information
type PDFTrailer struct {
	RootRef    string
	EncryptRef string
	InfoRef    string
	StartXRef  int64
}

// PDFObject represents a PDF object
type PDFObject struct {
	Number     int
	Generation int
	Offset     int64
	Content    []byte
}

// FormData represents the data to fill into the form
type FormData map[string]interface{}

// FormField represents a field in an XFA form
type FormField struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	FullName     string                 `json:"full_name"`
	Type         string                 `json:"type"`
	Value        interface{}            `json:"value,omitempty"`
	DefaultValue interface{}            `json:"default_value,omitempty"`
	Options      []string               `json:"options,omitempty"`
	Required     bool                   `json:"required"`
	ReadOnly     bool                   `json:"read_only"`
	PageNumber   int                    `json:"page_number,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
}

// FormStructure represents the complete extracted form structure
type FormStructure struct {
	Fields   []FormField `json:"fields"`
	Metadata struct {
		TotalFields int    `json:"total_fields"`
		TotalPages  int    `json:"total_pages"`
		FormType    string `json:"form_type,omitempty"`
	} `json:"metadata"`
}
