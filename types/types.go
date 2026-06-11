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
	O               []byte // Owner password hash (V1-V4) or encrypted owner key (V5+)
	U               []byte // User password hash (V1-V4) or encrypted user key (V5+)
	UE              []byte // Encrypted user encryption key (V5+, AES-256)
	OE              []byte // Encrypted owner encryption key (V5+, AES-256)
	P               int32  // Permissions
	EncryptMetadata bool
	EncryptKey      []byte // Master encryption key
	// StrFIdentity is true when the string crypt filter (/StrF) is /Identity,
	// meaning string values are stored unencrypted even though streams are
	// encrypted. Read and write paths must honor this and leave such strings
	// untouched. pdfer's own EncryptPDF declares /StrF /StdCF and encrypts
	// strings accordingly. (pdfer <= v2.5.0 declared /StrF /Identity while
	// still encrypting strings; strings in files it produced are no longer
	// transparently readable now that the declaration is honored — see GAPS.md.)
	StrFIdentity bool
	// EncryptObjNum is the object number of the /Encrypt dictionary itself.
	// Strings inside that dictionary (/O, /U, ...) are never encrypted (ISO
	// 32000-1 §7.6.5), so read and write paths must skip string crypto for
	// this object. Zero when unknown (object 0 cannot hold a dictionary).
	EncryptObjNum int
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
