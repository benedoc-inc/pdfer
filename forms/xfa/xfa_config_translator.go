package xfa

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseXFAConfig parses XFA config XML and converts it to XFAConfig type
func ParseXFAConfig(xfaConfigXML string, verbose bool) (*types.XFAConfig, error) {
	if verbose {
		log.Printf("Parsing XFA config XML (length: %d bytes)", len(xfaConfigXML))
	}

	config := &types.XFAConfig{
		Properties: make(map[string]interface{}),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaConfigXML))
	decoder.Strict = false

	var currentSection string
	var currentValue strings.Builder

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if verbose {
				log.Printf("XML parse error (continuing): %v", err)
			}
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			localName := se.Name.Local

			switch localName {
			case "config":
				// Root element
			case "present":
				currentSection = "present"
				if config.Present == nil {
					config.Present = &types.XFAConfigPresent{}
				}
				parsePresentAttributes(se, config.Present)
			case "validate":
				currentSection = "validate"
				if config.Validate == nil {
					config.Validate = &types.XFAConfigValidate{}
				}
			case "submit":
				currentSection = "submit"
				if config.Submit == nil {
					config.Submit = &types.XFAConfigSubmit{
						Properties: make(map[string]string),
					}
				}
				parseSubmitAttributes(se, config.Submit)
			case "destination":
				currentSection = "destination"
				if config.Destination == nil {
					config.Destination = &types.XFAConfigDestination{}
				}
				parseDestinationAttributes(se, config.Destination)
			case "acrobat":
				currentSection = "acrobat"
				if config.Acrobat == nil {
					config.Acrobat = &types.XFAConfigAcrobat{}
				}
				parseAcrobatAttributes(se, config.Acrobat)
			case "common":
				currentSection = "common"
				if config.Common == nil {
					config.Common = &types.XFAConfigCommon{}
				}
			case "output":
				currentSection = "output"
				if config.Output == nil {
					config.Output = &types.XFAConfigOutput{}
				}
			case "script":
				if currentSection == "present" {
					if config.Present.Script == nil {
						config.Present.Script = &types.XFAConfigScript{}
					}
				} else {
					if config.Script == nil {
						config.Script = &types.XFAConfigScript{}
					}
				}
			case "pdf":
				if currentSection == "present" && config.Present != nil {
					if config.Present.PDF == nil {
						config.Present.PDF = &types.XFAConfigPDF{}
					}
					parsePDFAttributes(se, config.Present.PDF)
				}
			case "xdp":
				if currentSection == "present" && config.Present != nil {
					if config.Present.XDP == nil {
						config.Present.XDP = &types.XFAConfigXDP{}
					}
				}
			case "print":
				if currentSection == "present" && config.Present != nil {
					if config.Present.Print == nil {
						config.Present.Print = &types.XFAConfigPrint{}
					}
				}
			case "interactive":
				if currentSection == "present" && config.Present != nil {
					if config.Present.Interactive == nil {
						config.Present.Interactive = &types.XFAConfigInteractive{}
					}
					parseInteractiveAttributes(se, config.Present.Interactive)
				}
			default:
				// Store as property
				currentValue.Reset()
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "present", "validate", "submit", "destination", "acrobat", "common", "output":
				currentSection = ""
			default:
				value := strings.TrimSpace(currentValue.String())
				if value != "" && currentSection != "" {
					// Store property
					if config.Properties == nil {
						config.Properties = make(map[string]interface{})
					}
					config.Properties[localName] = value
				}
				currentValue.Reset()
			}

		case xml.CharData:
			currentValue.WriteString(string(se))
		}
	}

	if verbose {
		log.Printf("Parsed XFA config with %d properties", len(config.Properties))
	}

	return config, nil
}

func parsePresentAttributes(se xml.StartElement, present *types.XFAConfigPresent) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "renderPolicy":
			present.RenderPolicy = attr.Value
		case "effectiveOutputPolicy":
			present.EffectiveOutputPolicy = attr.Value
		}
	}
}

func parseSubmitAttributes(se xml.StartElement, submit *types.XFAConfigSubmit) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "format":
			submit.Format = attr.Value
		case "target":
			submit.Target = attr.Value
		case "method":
			submit.Method = attr.Value
		case "embedPDF":
			submit.EmbedPDF = parseBool(attr.Value)
		case "xdpContent":
			submit.XDPContent = attr.Value
		case "textEncoding":
			submit.TextEncoding = attr.Value
		default:
			if submit.Properties == nil {
				submit.Properties = make(map[string]string)
			}
			submit.Properties[attr.Name.Local] = attr.Value
		}
	}
}

func parseDestinationAttributes(se xml.StartElement, dest *types.XFAConfigDestination) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "type":
			dest.Type = attr.Value
		case "target":
			dest.Target = attr.Value
		}
	}
}

func parseAcrobatAttributes(se xml.StartElement, acrobat *types.XFAConfigAcrobat) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "autoSave":
			acrobat.AutoSave = parseBool(attr.Value)
		case "autoSaveTime":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				acrobat.AutoSaveTime = val
			}
		case "formType":
			acrobat.FormType = attr.Value
		case "version":
			acrobat.Version = attr.Value
		}
	}
}

func parsePDFAttributes(se xml.StartElement, pdf *types.XFAConfigPDF) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "version":
			pdf.Version = attr.Value
		case "compression":
			pdf.Compression = attr.Value
		case "linearized":
			pdf.Linearized = parseBool(attr.Value)
		case "encryption":
			pdf.Encryption = attr.Value
		case "embedFonts":
			pdf.EmbedFonts = parseBool(attr.Value)
		case "tagged":
			pdf.Tagged = parseBool(attr.Value)
		case "accessibility":
			pdf.Accessibility = parseBool(attr.Value)
		}
	}
}

func parseInteractiveAttributes(se xml.StartElement, interactive *types.XFAConfigInteractive) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "currentPage":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				interactive.CurrentPage = val
			}
		case "zoom":
			interactive.Zoom = attr.Value
		case "viewMode":
			interactive.ViewMode = attr.Value
		}
	}
}

// ConfigToXFA converts XFAConfig to XFA XML
func ConfigToXFA(config *types.XFAConfig, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting XFAConfig to XFA XML")
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start config element
	configStart := xml.StartElement{Name: xml.Name{Local: "config"}}
	if err := enc.EncodeToken(configStart); err != nil {
		return "", fmt.Errorf("failed to encode config start: %v", err)
	}

	// Encode present section
	if config.Present != nil {
		if err := encodeConfigPresent(enc, config.Present); err != nil {
			return "", fmt.Errorf("failed to encode present: %v", err)
		}
	}

	// Encode validate section
	if config.Validate != nil {
		if err := encodeConfigValidate(enc, config.Validate); err != nil {
			return "", fmt.Errorf("failed to encode validate: %v", err)
		}
	}

	// Encode submit section
	if config.Submit != nil {
		if err := encodeConfigSubmit(enc, config.Submit); err != nil {
			return "", fmt.Errorf("failed to encode submit: %v", err)
		}
	}

	// Encode destination section
	if config.Destination != nil {
		if err := encodeConfigDestination(enc, config.Destination); err != nil {
			return "", fmt.Errorf("failed to encode destination: %v", err)
		}
	}

	// Encode acrobat section
	if config.Acrobat != nil {
		if err := encodeConfigAcrobat(enc, config.Acrobat); err != nil {
			return "", fmt.Errorf("failed to encode acrobat: %v", err)
		}
	}

	// Encode common section
	if config.Common != nil {
		if err := encodeConfigCommon(enc, config.Common); err != nil {
			return "", fmt.Errorf("failed to encode common: %v", err)
		}
	}

	// Encode output section
	if config.Output != nil {
		if err := encodeConfigOutput(enc, config.Output); err != nil {
			return "", fmt.Errorf("failed to encode output: %v", err)
		}
	}

	// Encode additional properties
	if config.Properties != nil {
		for key, value := range config.Properties {
			elemStart := xml.StartElement{Name: xml.Name{Local: key}}
			if err := enc.EncodeToken(elemStart); err != nil {
				return "", err
			}
			if err := enc.EncodeToken(xml.CharData(fmt.Sprintf("%v", value))); err != nil {
				return "", err
			}
			if err := enc.EncodeToken(elemStart.End()); err != nil {
				return "", err
			}
		}
	}

	// End config element
	if err := enc.EncodeToken(configStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode config end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted XFAConfig to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

func encodeConfigPresent(enc *xml.Encoder, present *types.XFAConfigPresent) error {
	presentStart := xml.StartElement{Name: xml.Name{Local: "present"}}
	attrs := []xml.Attr{}
	if present.RenderPolicy != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "renderPolicy"}, Value: present.RenderPolicy})
	}
	if present.EffectiveOutputPolicy != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "effectiveOutputPolicy"}, Value: present.EffectiveOutputPolicy})
	}
	presentStart.Attr = attrs

	if err := enc.EncodeToken(presentStart); err != nil {
		return err
	}

	if present.PDF != nil {
		if err := encodeConfigPDF(enc, present.PDF); err != nil {
			return err
		}
	}

	if present.Interactive != nil {
		if err := encodeConfigInteractive(enc, present.Interactive); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(presentStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigPDF(enc *xml.Encoder, pdf *types.XFAConfigPDF) error {
	pdfStart := xml.StartElement{Name: xml.Name{Local: "pdf"}}
	attrs := []xml.Attr{}
	if pdf.Version != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "version"}, Value: pdf.Version})
	}
	if pdf.Compression != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "compression"}, Value: pdf.Compression})
	}
	if pdf.Linearized {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "linearized"}, Value: "1"})
	}
	if pdf.Encryption != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "encryption"}, Value: pdf.Encryption})
	}
	if pdf.EmbedFonts {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "embedFonts"}, Value: "1"})
	}
	if pdf.Tagged {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "tagged"}, Value: "1"})
	}
	if pdf.Accessibility {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "accessibility"}, Value: "1"})
	}
	pdfStart.Attr = attrs

	if err := enc.EncodeToken(pdfStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(pdfStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigInteractive(enc *xml.Encoder, interactive *types.XFAConfigInteractive) error {
	interactiveStart := xml.StartElement{Name: xml.Name{Local: "interactive"}}
	attrs := []xml.Attr{}
	if interactive.CurrentPage > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "currentPage"}, Value: strconv.Itoa(interactive.CurrentPage)})
	}
	if interactive.Zoom != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "zoom"}, Value: interactive.Zoom})
	}
	if interactive.ViewMode != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "viewMode"}, Value: interactive.ViewMode})
	}
	interactiveStart.Attr = attrs

	if err := enc.EncodeToken(interactiveStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(interactiveStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigValidate(enc *xml.Encoder, validate *types.XFAConfigValidate) error {
	validateStart := xml.StartElement{Name: xml.Name{Local: "validate"}}
	attrs := []xml.Attr{}
	if validate.ScriptTest != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "scriptTest"}, Value: validate.ScriptTest})
	}
	if validate.NullTest != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "nullTest"}, Value: validate.NullTest})
	}
	if validate.FormatTest != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "formatTest"}, Value: validate.FormatTest})
	}
	validateStart.Attr = attrs

	if err := enc.EncodeToken(validateStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(validateStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigSubmit(enc *xml.Encoder, submit *types.XFAConfigSubmit) error {
	submitStart := xml.StartElement{Name: xml.Name{Local: "submit"}}
	attrs := []xml.Attr{}
	if submit.Format != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "format"}, Value: submit.Format})
	}
	if submit.Target != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "target"}, Value: submit.Target})
	}
	if submit.Method != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "method"}, Value: submit.Method})
	}
	if submit.EmbedPDF {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "embedPDF"}, Value: "1"})
	}
	if submit.XDPContent != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "xdpContent"}, Value: submit.XDPContent})
	}
	if submit.TextEncoding != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "textEncoding"}, Value: submit.TextEncoding})
	}
	submitStart.Attr = attrs

	if err := enc.EncodeToken(submitStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(submitStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigDestination(enc *xml.Encoder, dest *types.XFAConfigDestination) error {
	destStart := xml.StartElement{Name: xml.Name{Local: "destination"}}
	attrs := []xml.Attr{}
	if dest.Type != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "type"}, Value: dest.Type})
	}
	if dest.Target != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "target"}, Value: dest.Target})
	}
	destStart.Attr = attrs

	if err := enc.EncodeToken(destStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(destStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigAcrobat(enc *xml.Encoder, acrobat *types.XFAConfigAcrobat) error {
	acrobatStart := xml.StartElement{Name: xml.Name{Local: "acrobat"}}
	attrs := []xml.Attr{}
	if acrobat.AutoSave {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "autoSave"}, Value: "1"})
	}
	if acrobat.AutoSaveTime > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "autoSaveTime"}, Value: strconv.Itoa(acrobat.AutoSaveTime)})
	}
	if acrobat.FormType != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "formType"}, Value: acrobat.FormType})
	}
	if acrobat.Version != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "version"}, Value: acrobat.Version})
	}
	acrobatStart.Attr = attrs

	if err := enc.EncodeToken(acrobatStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(acrobatStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigCommon(enc *xml.Encoder, common *types.XFAConfigCommon) error {
	commonStart := xml.StartElement{Name: xml.Name{Local: "common"}}
	attrs := []xml.Attr{}
	if common.DataPrefix != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "dataPrefix"}, Value: common.DataPrefix})
	}
	if common.TimeStamp != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "timeStamp"}, Value: common.TimeStamp})
	}
	commonStart.Attr = attrs

	if err := enc.EncodeToken(commonStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(commonStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeConfigOutput(enc *xml.Encoder, output *types.XFAConfigOutput) error {
	outputStart := xml.StartElement{Name: xml.Name{Local: "output"}}
	attrs := []xml.Attr{}
	if output.Format != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "format"}, Value: output.Format})
	}
	if output.Destination != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "destination"}, Value: output.Destination})
	}
	outputStart.Attr = attrs

	if err := enc.EncodeToken(outputStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(outputStart.End()); err != nil {
		return err
	}

	return nil
}
