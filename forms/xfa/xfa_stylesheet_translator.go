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

// ParseXFAStylesheet parses XFA stylesheet XML and converts it to XFAStylesheet type
func ParseXFAStylesheet(xfaStylesheetXML string, verbose bool) (*types.XFAStylesheet, error) {
	if verbose {
		log.Printf("Parsing XFA stylesheet XML (length: %d bytes)", len(xfaStylesheetXML))
	}

	stylesheet := &types.XFAStylesheet{
		Styles: make([]types.XFAStyle, 0),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaStylesheetXML))
	decoder.Strict = false

	var currentStyle *types.XFAStyle
	var currentProperty string
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
			case "stylesheet":
				// Root element
			case "style":
				currentStyle = &types.XFAStyle{
					Properties: make(map[string]interface{}),
				}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						currentStyle.Name = attr.Value
					case "type":
						currentStyle.Type = attr.Value
					}
				}
			case "font":
				if currentStyle != nil {
					currentProperty = "font"
					parseFontAttributes(se, currentStyle)
				}
			case "border":
				if currentStyle != nil {
					currentProperty = "border"
					parseBorderAttributes(se, currentStyle)
				}
			case "margin":
				if currentStyle != nil {
					currentProperty = "margin"
					if currentStyle.Properties["margin"] == nil {
						currentStyle.Properties["margin"] = &types.XFAMargin{}
					}
					parseMarginAttributes(se, currentStyle.Properties["margin"].(*types.XFAMargin))
				}
			case "padding":
				if currentStyle != nil {
					currentProperty = "padding"
					if currentStyle.Properties["padding"] == nil {
						currentStyle.Properties["padding"] = &types.XFAPadding{}
					}
					parsePaddingAttributes(se, currentStyle.Properties["padding"].(*types.XFAPadding))
				}
			case "position":
				if currentStyle != nil {
					currentProperty = "position"
					if currentStyle.Properties["position"] == nil {
						currentStyle.Properties["position"] = &types.XFAPosition{}
					}
					parsePositionAttributes(se, currentStyle.Properties["position"].(*types.XFAPosition))
				}
			case "size":
				if currentStyle != nil {
					currentProperty = "size"
					if currentStyle.Properties["size"] == nil {
						currentStyle.Properties["size"] = &types.XFASize{}
					}
					parseSizeAttributes(se, currentStyle.Properties["size"].(*types.XFASize))
				}
			default:
				if currentStyle != nil {
					currentProperty = localName
					currentValue.Reset()
				}
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "style":
				if currentStyle != nil {
					stylesheet.Styles = append(stylesheet.Styles, *currentStyle)
					if verbose {
						log.Printf("Parsed style: %s (type: %s)", currentStyle.Name, currentStyle.Type)
					}
				}
				currentStyle = nil
				currentProperty = ""
			case "font", "border", "margin", "padding", "position", "size":
				currentProperty = ""
			default:
				if currentStyle != nil && currentProperty != "" {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						parsedValue := parseStyleValue(value)
						currentStyle.Properties[localName] = parsedValue
					}
				}
				currentValue.Reset()
			}

		case xml.CharData:
			if currentStyle != nil && currentProperty != "" {
				currentValue.WriteString(string(se))
			}
		}
	}

	if verbose {
		log.Printf("Parsed XFA stylesheet: %d styles", len(stylesheet.Styles))
	}

	return stylesheet, nil
}

func parseFontAttributes(se xml.StartElement, style *types.XFAStyle) {
	font := &types.XFAStyleFont{}
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "family":
			font.Family = attr.Value
		case "size":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				font.Size = val
			}
		case "weight":
			font.Weight = attr.Value
		case "style":
			font.Style = attr.Value
		case "color":
			font.Color = attr.Value
		case "decoration":
			font.Decoration = attr.Value
		}
	}
	style.Properties["font"] = font
}

func parseBorderAttributes(se xml.StartElement, style *types.XFAStyle) {
	border := &types.XFAStyleBorder{}
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "width":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				border.Width = val
			}
		case "style":
			border.Style = attr.Value
		case "color":
			border.Color = attr.Value
		case "radius":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				border.Radius = val
			}
		}
	}
	style.Properties["border"] = border
}

func parseMarginAttributes(se xml.StartElement, margin *types.XFAMargin) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "top":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				margin.Top = val
			}
		case "right":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				margin.Right = val
			}
		case "bottom":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				margin.Bottom = val
			}
		case "left":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				margin.Left = val
			}
		}
	}
}

func parsePaddingAttributes(se xml.StartElement, padding *types.XFAPadding) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "top":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				padding.Top = val
			}
		case "right":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				padding.Right = val
			}
		case "bottom":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				padding.Bottom = val
			}
		case "left":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				padding.Left = val
			}
		}
	}
}

func parsePositionAttributes(se xml.StartElement, position *types.XFAPosition) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "x":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				position.X = val
			}
		case "y":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				position.Y = val
			}
		}
	}
}

func parseSizeAttributes(se xml.StartElement, size *types.XFASize) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "width":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				size.Width = val
			}
		case "height":
			if val, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				size.Height = val
			}
		}
	}
}

func parseStyleValue(value string) interface{} {
	// Try to parse as number
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}
	// Try boolean
	if value == "true" || value == "1" {
		return true
	}
	if value == "false" || value == "0" {
		return false
	}
	// Return as string
	return value
}

// StylesheetToXFA converts XFAStylesheet to XFA XML
func StylesheetToXFA(stylesheet *types.XFAStylesheet, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting XFAStylesheet to XFA XML: %d styles", len(stylesheet.Styles))
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start stylesheet element
	stylesheetStart := xml.StartElement{Name: xml.Name{Local: "stylesheet"}}
	if err := enc.EncodeToken(stylesheetStart); err != nil {
		return "", fmt.Errorf("failed to encode stylesheet start: %v", err)
	}

	// Encode styles
	for _, style := range stylesheet.Styles {
		if err := encodeStyle(enc, style); err != nil {
			return "", fmt.Errorf("failed to encode style %s: %v", style.Name, err)
		}
	}

	// End stylesheet element
	if err := enc.EncodeToken(stylesheetStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode stylesheet end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted XFAStylesheet to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

func encodeStyle(enc *xml.Encoder, style types.XFAStyle) error {
	styleStart := xml.StartElement{Name: xml.Name{Local: "style"}}
	attrs := []xml.Attr{}
	if style.Name != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "name"}, Value: style.Name})
	}
	if style.Type != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "type"}, Value: style.Type})
	}
	styleStart.Attr = attrs

	if err := enc.EncodeToken(styleStart); err != nil {
		return err
	}

	// Encode font
	if font, ok := style.Properties["font"].(*types.XFAStyleFont); ok && font != nil {
		if err := encodeStyleFont(enc, font); err != nil {
			return err
		}
	}

	// Encode border
	if border, ok := style.Properties["border"].(*types.XFAStyleBorder); ok && border != nil {
		if err := encodeStyleBorder(enc, border); err != nil {
			return err
		}
	}

	// Encode margin
	if margin, ok := style.Properties["margin"].(*types.XFAMargin); ok && margin != nil {
		if err := encodeMargin(enc, margin); err != nil {
			return err
		}
	}

	// Encode padding
	if padding, ok := style.Properties["padding"].(*types.XFAPadding); ok && padding != nil {
		if err := encodePadding(enc, padding); err != nil {
			return err
		}
	}

	// Encode position
	if position, ok := style.Properties["position"].(*types.XFAPosition); ok && position != nil {
		if err := encodePosition(enc, position); err != nil {
			return err
		}
	}

	// Encode size
	if size, ok := style.Properties["size"].(*types.XFASize); ok && size != nil {
		if err := encodeSize(enc, size); err != nil {
			return err
		}
	}

	// Encode other properties
	for key, value := range style.Properties {
		if key != "font" && key != "border" && key != "margin" && key != "padding" && key != "position" && key != "size" {
			elemStart := xml.StartElement{Name: xml.Name{Local: key}}
			if err := enc.EncodeToken(elemStart); err != nil {
				return err
			}
			if err := enc.EncodeToken(xml.CharData(fmt.Sprintf("%v", value))); err != nil {
				return err
			}
			if err := enc.EncodeToken(elemStart.End()); err != nil {
				return err
			}
		}
	}

	if err := enc.EncodeToken(styleStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeStyleFont(enc *xml.Encoder, font *types.XFAStyleFont) error {
	fontStart := xml.StartElement{Name: xml.Name{Local: "font"}}
	attrs := []xml.Attr{}
	if font.Family != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "family"}, Value: font.Family})
	}
	if font.Size > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "size"}, Value: strconv.FormatFloat(font.Size, 'f', -1, 64)})
	}
	if font.Weight != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "weight"}, Value: font.Weight})
	}
	if font.Style != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: font.Style})
	}
	if font.Color != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "color"}, Value: font.Color})
	}
	if font.Decoration != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "decoration"}, Value: font.Decoration})
	}
	fontStart.Attr = attrs

	if err := enc.EncodeToken(fontStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(fontStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeStyleBorder(enc *xml.Encoder, border *types.XFAStyleBorder) error {
	borderStart := xml.StartElement{Name: xml.Name{Local: "border"}}
	attrs := []xml.Attr{}
	if border.Width > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "width"}, Value: strconv.FormatFloat(border.Width, 'f', -1, 64)})
	}
	if border.Style != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "style"}, Value: border.Style})
	}
	if border.Color != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "color"}, Value: border.Color})
	}
	if border.Radius > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "radius"}, Value: strconv.FormatFloat(border.Radius, 'f', -1, 64)})
	}
	borderStart.Attr = attrs

	if err := enc.EncodeToken(borderStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(borderStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeMargin(enc *xml.Encoder, margin *types.XFAMargin) error {
	marginStart := xml.StartElement{Name: xml.Name{Local: "margin"}}
	attrs := []xml.Attr{}
	if margin.Top > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "top"}, Value: strconv.FormatFloat(margin.Top, 'f', -1, 64)})
	}
	if margin.Right > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "right"}, Value: strconv.FormatFloat(margin.Right, 'f', -1, 64)})
	}
	if margin.Bottom > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "bottom"}, Value: strconv.FormatFloat(margin.Bottom, 'f', -1, 64)})
	}
	if margin.Left > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "left"}, Value: strconv.FormatFloat(margin.Left, 'f', -1, 64)})
	}
	marginStart.Attr = attrs

	if err := enc.EncodeToken(marginStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(marginStart.End()); err != nil {
		return err
	}

	return nil
}

func encodePadding(enc *xml.Encoder, padding *types.XFAPadding) error {
	paddingStart := xml.StartElement{Name: xml.Name{Local: "padding"}}
	attrs := []xml.Attr{}
	if padding.Top > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "top"}, Value: strconv.FormatFloat(padding.Top, 'f', -1, 64)})
	}
	if padding.Right > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "right"}, Value: strconv.FormatFloat(padding.Right, 'f', -1, 64)})
	}
	if padding.Bottom > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "bottom"}, Value: strconv.FormatFloat(padding.Bottom, 'f', -1, 64)})
	}
	if padding.Left > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "left"}, Value: strconv.FormatFloat(padding.Left, 'f', -1, 64)})
	}
	paddingStart.Attr = attrs

	if err := enc.EncodeToken(paddingStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(paddingStart.End()); err != nil {
		return err
	}

	return nil
}

func encodePosition(enc *xml.Encoder, position *types.XFAPosition) error {
	positionStart := xml.StartElement{Name: xml.Name{Local: "position"}}
	attrs := []xml.Attr{}
	if position.X > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "x"}, Value: strconv.FormatFloat(position.X, 'f', -1, 64)})
	}
	if position.Y > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "y"}, Value: strconv.FormatFloat(position.Y, 'f', -1, 64)})
	}
	positionStart.Attr = attrs

	if err := enc.EncodeToken(positionStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(positionStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeSize(enc *xml.Encoder, size *types.XFASize) error {
	sizeStart := xml.StartElement{Name: xml.Name{Local: "size"}}
	attrs := []xml.Attr{}
	if size.Width > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "width"}, Value: strconv.FormatFloat(size.Width, 'f', -1, 64)})
	}
	if size.Height > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "height"}, Value: strconv.FormatFloat(size.Height, 'f', -1, 64)})
	}
	sizeStart.Attr = attrs

	if err := enc.EncodeToken(sizeStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(sizeStart.End()); err != nil {
		return err
	}

	return nil
}
