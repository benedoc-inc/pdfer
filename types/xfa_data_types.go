package types

// XFADatasets represents the data values in an XFA form
// This contains the actual form data (user inputs or external data)
type XFADatasets struct {
	Fields map[string]interface{} `json:"fields"`           // Field name -> value mapping
	Groups []XFAFieldGroup        `json:"groups,omitempty"` // Hierarchical groups of fields
}

// XFAFieldGroup represents a group of related fields in XFA datasets
type XFAFieldGroup struct {
	Name   string                 `json:"name"`
	Fields map[string]interface{} `json:"fields"`
	Groups []XFAFieldGroup        `json:"groups,omitempty"` // Nested groups
}

// XFAConfig represents configuration settings for the XFA processor
type XFAConfig struct {
	Present     *XFAConfigPresent      `json:"present,omitempty"`     // Presentation settings
	Validate    *XFAConfigValidate     `json:"validate,omitempty"`    // Validation settings
	Submit      *XFAConfigSubmit       `json:"submit,omitempty"`      // Submit settings
	Destination *XFAConfigDestination  `json:"destination,omitempty"` // Destination settings
	Acrobat     *XFAConfigAcrobat      `json:"acrobat,omitempty"`     // Acrobat-specific settings
	Common      *XFAConfigCommon       `json:"common,omitempty"`      // Common settings
	Output      *XFAConfigOutput       `json:"output,omitempty"`      // Output settings
	Script      *XFAConfigScript       `json:"script,omitempty"`      // Script settings
	Trace       *XFAConfigTrace        `json:"trace,omitempty"`       // Trace/debug settings
	Properties  map[string]interface{} `json:"properties,omitempty"`  // Additional properties
}

// XFAConfigPresent represents presentation configuration
type XFAConfigPresent struct {
	PDF                   *XFAConfigPDF         `json:"pdf,omitempty"`
	XDP                   *XFAConfigXDP         `json:"xdp,omitempty"`
	Print                 *XFAConfigPrint       `json:"print,omitempty"`
	Interactive           *XFAConfigInteractive `json:"interactive,omitempty"`
	Script                *XFAConfigScript      `json:"script,omitempty"`
	RenderPolicy          string                `json:"render_policy,omitempty"` // "client" or "server"
	EffectiveOutputPolicy string                `json:"effective_output_policy,omitempty"`
}

// XFAConfigPDF represents PDF-specific presentation settings
type XFAConfigPDF struct {
	Version       string `json:"version,omitempty"`       // PDF version
	Compression   string `json:"compression,omitempty"`   // Compression level
	Linearized    bool   `json:"linearized,omitempty"`    // Linearized PDF
	Encryption    string `json:"encryption,omitempty"`    // Encryption method
	EmbedFonts    bool   `json:"embed_fonts,omitempty"`   // Embed fonts
	Tagged        bool   `json:"tagged,omitempty"`        // Tagged PDF
	Accessibility bool   `json:"accessibility,omitempty"` // Accessibility features
}

// XFAConfigXDP represents XDP-specific settings
type XFAConfigXDP struct {
	Version string `json:"version,omitempty"`
}

// XFAConfigPrint represents print settings
type XFAConfigPrint struct {
	PrinterName string `json:"printer_name,omitempty"`
	PrinterCap  string `json:"printer_cap,omitempty"`
}

// XFAConfigInteractive represents interactive form settings
type XFAConfigInteractive struct {
	CurrentPage int    `json:"current_page,omitempty"`
	Zoom        string `json:"zoom,omitempty"`
	ViewMode    string `json:"view_mode,omitempty"` // "continuous", "single", etc.
}

// XFAConfigValidate represents validation configuration
type XFAConfigValidate struct {
	ScriptTest string `json:"script_test,omitempty"`
	NullTest   string `json:"null_test,omitempty"`
	FormatTest string `json:"format_test,omitempty"`
}

// XFAConfigSubmit represents submit configuration
type XFAConfigSubmit struct {
	Format       string            `json:"format,omitempty"`      // "pdf", "xdp", "xml", "html", etc.
	Target       string            `json:"target,omitempty"`      // Target URL
	Method       string            `json:"method,omitempty"`      // "post", "get", "put"
	EmbedPDF     bool              `json:"embed_pdf,omitempty"`   // Embed PDF in submission
	XDPContent   string            `json:"xdp_content,omitempty"` // XDP content type
	TextEncoding string            `json:"text_encoding,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

// XFAConfigDestination represents destination configuration
type XFAConfigDestination struct {
	Type   string `json:"type,omitempty"`   // "email", "file", "printer", etc.
	Target string `json:"target,omitempty"` // Target destination
}

// XFAConfigAcrobat represents Acrobat-specific settings
type XFAConfigAcrobat struct {
	AutoSave     bool   `json:"auto_save,omitempty"`
	AutoSaveTime int    `json:"auto_save_time,omitempty"` // Seconds
	FormType     string `json:"form_type,omitempty"`
	Version      string `json:"version,omitempty"`
}

// XFAConfigCommon represents common configuration
type XFAConfigCommon struct {
	DataPrefix string `json:"data_prefix,omitempty"`
	TimeStamp  string `json:"time_stamp,omitempty"`
}

// XFAConfigOutput represents output configuration
type XFAConfigOutput struct {
	Format      string `json:"format,omitempty"`
	Destination string `json:"destination,omitempty"`
}

// XFAConfigScript represents script configuration
type XFAConfigScript struct {
	ContentType string `json:"content_type,omitempty"`
	RunAt       string `json:"run_at,omitempty"` // "client", "server", "both"
}

// XFAConfigTrace represents trace/debug configuration
type XFAConfigTrace struct {
	Enabled bool   `json:"enabled,omitempty"`
	Level   string `json:"level,omitempty"` // "error", "warning", "info", "debug"
}

// XFALocaleSet represents localization settings for the form
type XFALocaleSet struct {
	Locales []XFALocale `json:"locales"`
	Default string      `json:"default,omitempty"` // Default locale code
}

// XFALocale represents a specific locale configuration
type XFALocale struct {
	Code          string            `json:"code"`                     // Locale code (e.g., "en_US", "fr_FR")
	Name          string            `json:"name,omitempty"`           // Locale name
	Calendar      *XFACalendar      `json:"calendar,omitempty"`       // Calendar settings
	Currency      *XFACurrency      `json:"currency,omitempty"`       // Currency settings
	DatePattern   *XFADatePattern   `json:"date_pattern,omitempty"`   // Date format
	TimePattern   *XFATimePattern   `json:"time_pattern,omitempty"`   // Time format
	NumberPattern *XFANumberPattern `json:"number_pattern,omitempty"` // Number format
	Text          *XFAText          `json:"text,omitempty"`           // Text settings
}

// XFACalendar represents calendar settings
type XFACalendar struct {
	Symbols  string   `json:"symbols,omitempty"`   // Calendar symbols
	FirstDay string   `json:"first_day,omitempty"` // First day of week
	Weekend  string   `json:"weekend,omitempty"`   // Weekend days
	Holidays []string `json:"holidays,omitempty"`  // Holiday list
}

// XFACurrency represents currency settings
type XFACurrency struct {
	Symbol    string `json:"symbol,omitempty"`    // Currency symbol ($, €, etc.)
	Name      string `json:"name,omitempty"`      // Currency name
	Precision int    `json:"precision,omitempty"` // Decimal precision
}

// XFADatePattern represents date format pattern
type XFADatePattern struct {
	Format string `json:"format,omitempty"` // Date format string
	Symbol string `json:"symbol,omitempty"` // Date symbol set
}

// XFATimePattern represents time format pattern
type XFATimePattern struct {
	Format string `json:"format,omitempty"` // Time format string
	Symbol string `json:"symbol,omitempty"` // Time symbol set
}

// XFANumberPattern represents number format pattern
type XFANumberPattern struct {
	Format    string `json:"format,omitempty"`    // Number format string
	Symbol    string `json:"symbol,omitempty"`    // Number symbol set
	Grouping  string `json:"grouping,omitempty"`  // Grouping separator
	Decimal   string `json:"decimal,omitempty"`   // Decimal separator
	Precision int    `json:"precision,omitempty"` // Decimal precision
}

// XFAText represents text settings
type XFAText struct {
	Direction string `json:"direction,omitempty"` // "ltr" or "rtl"
	Encoding  string `json:"encoding,omitempty"`  // Text encoding
}

// XFAConnectionSet represents data connection configurations
type XFAConnectionSet struct {
	Connections []XFAConnection `json:"connections"`
}

// XFAConnection represents a single data connection
type XFAConnection struct {
	Name            string                 `json:"name"`                       // Connection name
	Type            string                 `json:"type"`                       // Connection type: "http", "soap", "wsdl", "xml", "odbc", etc.
	DataDescription *XFADataDescription    `json:"data_description,omitempty"` // Data description
	Properties      map[string]interface{} `json:"properties,omitempty"`       // Connection properties
}

// XFADataDescription represents data description for connections
type XFADataDescription struct {
	Ref        string                 `json:"ref,omitempty"`       // Reference to data description
	Schema     string                 `json:"schema,omitempty"`    // Schema definition
	Namespace  string                 `json:"namespace,omitempty"` // XML namespace
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// XFAConnectionHTTP represents HTTP connection details
type XFAConnectionHTTP struct {
	URL        string            `json:"url"`
	Method     string            `json:"method,omitempty"` // "GET", "POST", etc.
	Headers    map[string]string `json:"headers,omitempty"`
	Timeout    int               `json:"timeout,omitempty"` // Timeout in seconds
	Credential *XFACredential    `json:"credential,omitempty"`
}

// XFAConnectionSOAP represents SOAP connection details
type XFAConnectionSOAP struct {
	WSDL       string            `json:"wsdl,omitempty"`
	Operation  string            `json:"operation,omitempty"`
	Endpoint   string            `json:"endpoint,omitempty"`
	Action     string            `json:"action,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Credential *XFACredential    `json:"credential,omitempty"`
}

// XFAConnectionODBC represents ODBC database connection details
type XFAConnectionODBC struct {
	DSN        string         `json:"dsn"` // Data source name
	Driver     string         `json:"driver,omitempty"`
	Server     string         `json:"server,omitempty"`
	Database   string         `json:"database,omitempty"`
	Query      string         `json:"query,omitempty"` // SQL query
	Credential *XFACredential `json:"credential,omitempty"`
}

// XFACredential represents authentication credentials
type XFACredential struct {
	Type     string `json:"type,omitempty"` // "basic", "digest", "ntlm", etc.
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Domain   string `json:"domain,omitempty"`
}

// XFAStylesheet represents styling information for the form
type XFAStylesheet struct {
	Styles []XFAStyle `json:"styles"`
}

// XFAStyle represents a single style definition
type XFAStyle struct {
	Name       string                 `json:"name"`                 // Style name
	Type       string                 `json:"type,omitempty"`       // Style type: "font", "color", "border", etc.
	Properties map[string]interface{} `json:"properties,omitempty"` // Style properties
}

// XFAStyleFont represents font styling
type XFAStyleFont struct {
	Family     string  `json:"family,omitempty"`     // Font family
	Size       float64 `json:"size,omitempty"`       // Font size
	Weight     string  `json:"weight,omitempty"`     // "normal", "bold", etc.
	Style      string  `json:"style,omitempty"`      // "normal", "italic", etc.
	Color      string  `json:"color,omitempty"`      // Font color
	Decoration string  `json:"decoration,omitempty"` // "none", "underline", etc.
}

// XFAStyleBorder represents border styling
type XFAStyleBorder struct {
	Width  float64 `json:"width,omitempty"`
	Style  string  `json:"style,omitempty"` // "solid", "dashed", "dotted", etc.
	Color  string  `json:"color,omitempty"`
	Radius float64 `json:"radius,omitempty"` // Border radius
}

// XFAStyleLayout represents layout styling
type XFAStyleLayout struct {
	Margin    *XFAMargin   `json:"margin,omitempty"`
	Padding   *XFAPadding  `json:"padding,omitempty"`
	Position  *XFAPosition `json:"position,omitempty"`
	Size      *XFASize     `json:"size,omitempty"`
	Alignment string       `json:"alignment,omitempty"` // "left", "center", "right", etc.
}

// XFAMargin represents margin settings
type XFAMargin struct {
	Top    float64 `json:"top,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Left   float64 `json:"left,omitempty"`
}

// XFAPadding represents padding settings
type XFAPadding struct {
	Top    float64 `json:"top,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Left   float64 `json:"left,omitempty"`
}

// XFAPosition represents position settings
type XFAPosition struct {
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
}

// XFASize represents size settings
type XFASize struct {
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}
