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

// ParseXFALocaleSet parses XFA localeSet XML and converts it to XFALocaleSet type
func ParseXFALocaleSet(xfaLocaleSetXML string, verbose bool) (*types.XFALocaleSet, error) {
	if verbose {
		log.Printf("Parsing XFA localeSet XML (length: %d bytes)", len(xfaLocaleSetXML))
	}

	localeSet := &types.XFALocaleSet{
		Locales: make([]types.XFALocale, 0),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaLocaleSetXML))
	decoder.Strict = false

	var currentLocale *types.XFALocale
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
			case "localeSet":
				// Extract default locale
				for _, attr := range se.Attr {
					if attr.Name.Local == "default" {
						localeSet.Default = attr.Value
					}
				}
			case "locale":
				currentLocale = &types.XFALocale{}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						currentLocale.Name = attr.Value
					case "code", "id":
						currentLocale.Code = attr.Value
					}
				}
			case "calendar":
				if currentLocale != nil {
					currentSection = "calendar"
					if currentLocale.Calendar == nil {
						currentLocale.Calendar = &types.XFACalendar{}
					}
					parseCalendarAttributes(se, currentLocale.Calendar)
				}
			case "currency":
				if currentLocale != nil {
					currentSection = "currency"
					if currentLocale.Currency == nil {
						currentLocale.Currency = &types.XFACurrency{}
					}
					parseCurrencyAttributes(se, currentLocale.Currency)
				}
			case "datePattern":
				if currentLocale != nil {
					currentSection = "datePattern"
					if currentLocale.DatePattern == nil {
						currentLocale.DatePattern = &types.XFADatePattern{}
					}
					parseDatePatternAttributes(se, currentLocale.DatePattern)
				}
			case "timePattern":
				if currentLocale != nil {
					currentSection = "timePattern"
					if currentLocale.TimePattern == nil {
						currentLocale.TimePattern = &types.XFATimePattern{}
					}
					parseTimePatternAttributes(se, currentLocale.TimePattern)
				}
			case "numberPattern":
				if currentLocale != nil {
					currentSection = "numberPattern"
					if currentLocale.NumberPattern == nil {
						currentLocale.NumberPattern = &types.XFANumberPattern{}
					}
					parseNumberPatternAttributes(se, currentLocale.NumberPattern)
				}
			case "text":
				if currentLocale != nil {
					currentSection = "text"
					if currentLocale.Text == nil {
						currentLocale.Text = &types.XFAText{}
					}
					parseTextAttributes(se, currentLocale.Text)
				}
			case "holiday":
				if currentSection == "calendar" && currentLocale != nil && currentLocale.Calendar != nil {
					currentValue.Reset()
				}
			default:
				currentValue.Reset()
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "locale":
				if currentLocale != nil {
					localeSet.Locales = append(localeSet.Locales, *currentLocale)
					if verbose {
						log.Printf("Parsed locale: %s (%s)", currentLocale.Code, currentLocale.Name)
					}
				}
				currentLocale = nil
				currentSection = ""
			case "calendar", "currency", "datePattern", "timePattern", "numberPattern", "text":
				currentSection = ""
			case "holiday":
				if currentSection == "calendar" && currentLocale != nil && currentLocale.Calendar != nil {
					holiday := strings.TrimSpace(currentValue.String())
					if holiday != "" {
						currentLocale.Calendar.Holidays = append(currentLocale.Calendar.Holidays, holiday)
					}
				}
			default:
				currentValue.Reset()
			}

		case xml.CharData:
			if currentSection == "calendar" && strings.Contains(string(se), "holiday") {
				// Holiday content
			} else {
				currentValue.WriteString(string(se))
			}
		}
	}

	if verbose {
		log.Printf("Parsed XFA localeSet: %d locales", len(localeSet.Locales))
	}

	return localeSet, nil
}

func parseCalendarAttributes(se xml.StartElement, calendar *types.XFACalendar) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "symbols":
			calendar.Symbols = attr.Value
		case "firstDay":
			calendar.FirstDay = attr.Value
		case "weekend":
			calendar.Weekend = attr.Value
		}
	}
}

func parseCurrencyAttributes(se xml.StartElement, currency *types.XFACurrency) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "symbol":
			currency.Symbol = attr.Value
		case "name":
			currency.Name = attr.Value
		case "precision":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				currency.Precision = val
			}
		}
	}
}

func parseDatePatternAttributes(se xml.StartElement, pattern *types.XFADatePattern) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "format":
			pattern.Format = attr.Value
		case "symbol":
			pattern.Symbol = attr.Value
		}
	}
}

func parseTimePatternAttributes(se xml.StartElement, pattern *types.XFATimePattern) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "format":
			pattern.Format = attr.Value
		case "symbol":
			pattern.Symbol = attr.Value
		}
	}
}

func parseNumberPatternAttributes(se xml.StartElement, pattern *types.XFANumberPattern) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "format":
			pattern.Format = attr.Value
		case "symbol":
			pattern.Symbol = attr.Value
		case "grouping":
			pattern.Grouping = attr.Value
		case "decimal":
			pattern.Decimal = attr.Value
		case "precision":
			if val, err := strconv.Atoi(attr.Value); err == nil {
				pattern.Precision = val
			}
		}
	}
}

func parseTextAttributes(se xml.StartElement, text *types.XFAText) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "direction":
			text.Direction = attr.Value
		case "encoding":
			text.Encoding = attr.Value
		}
	}
}

// LocaleSetToXFA converts XFALocaleSet to XFA XML
func LocaleSetToXFA(localeSet *types.XFALocaleSet, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting XFALocaleSet to XFA XML: %d locales", len(localeSet.Locales))
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start localeSet element
	localeSetStart := xml.StartElement{Name: xml.Name{Local: "localeSet"}}
	attrs := []xml.Attr{}
	if localeSet.Default != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "default"}, Value: localeSet.Default})
	}
	localeSetStart.Attr = attrs

	if err := enc.EncodeToken(localeSetStart); err != nil {
		return "", fmt.Errorf("failed to encode localeSet start: %v", err)
	}

	// Encode locales
	for _, locale := range localeSet.Locales {
		if err := encodeLocale(enc, locale); err != nil {
			return "", fmt.Errorf("failed to encode locale %s: %v", locale.Code, err)
		}
	}

	// End localeSet element
	if err := enc.EncodeToken(localeSetStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode localeSet end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted XFALocaleSet to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

func encodeLocale(enc *xml.Encoder, locale types.XFALocale) error {
	localeStart := xml.StartElement{Name: xml.Name{Local: "locale"}}
	attrs := []xml.Attr{}
	if locale.Code != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "code"}, Value: locale.Code})
	}
	if locale.Name != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "name"}, Value: locale.Name})
	}
	localeStart.Attr = attrs

	if err := enc.EncodeToken(localeStart); err != nil {
		return err
	}

	if locale.Calendar != nil {
		if err := encodeCalendar(enc, locale.Calendar); err != nil {
			return err
		}
	}

	if locale.Currency != nil {
		if err := encodeCurrency(enc, locale.Currency); err != nil {
			return err
		}
	}

	if locale.DatePattern != nil {
		if err := encodeDatePattern(enc, locale.DatePattern); err != nil {
			return err
		}
	}

	if locale.TimePattern != nil {
		if err := encodeTimePattern(enc, locale.TimePattern); err != nil {
			return err
		}
	}

	if locale.NumberPattern != nil {
		if err := encodeNumberPattern(enc, locale.NumberPattern); err != nil {
			return err
		}
	}

	if locale.Text != nil {
		if err := encodeText(enc, locale.Text); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(localeStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeCalendar(enc *xml.Encoder, calendar *types.XFACalendar) error {
	calendarStart := xml.StartElement{Name: xml.Name{Local: "calendar"}}
	attrs := []xml.Attr{}
	if calendar.Symbols != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "symbols"}, Value: calendar.Symbols})
	}
	if calendar.FirstDay != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "firstDay"}, Value: calendar.FirstDay})
	}
	if calendar.Weekend != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "weekend"}, Value: calendar.Weekend})
	}
	calendarStart.Attr = attrs

	if err := enc.EncodeToken(calendarStart); err != nil {
		return err
	}

	// Encode holidays
	for _, holiday := range calendar.Holidays {
		holidayStart := xml.StartElement{Name: xml.Name{Local: "holiday"}}
		if err := enc.EncodeToken(holidayStart); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(holiday)); err != nil {
			return err
		}
		if err := enc.EncodeToken(holidayStart.End()); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(calendarStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeCurrency(enc *xml.Encoder, currency *types.XFACurrency) error {
	currencyStart := xml.StartElement{Name: xml.Name{Local: "currency"}}
	attrs := []xml.Attr{}
	if currency.Symbol != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "symbol"}, Value: currency.Symbol})
	}
	if currency.Name != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "name"}, Value: currency.Name})
	}
	if currency.Precision > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "precision"}, Value: strconv.Itoa(currency.Precision)})
	}
	currencyStart.Attr = attrs

	if err := enc.EncodeToken(currencyStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(currencyStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeDatePattern(enc *xml.Encoder, pattern *types.XFADatePattern) error {
	patternStart := xml.StartElement{Name: xml.Name{Local: "datePattern"}}
	attrs := []xml.Attr{}
	if pattern.Format != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "format"}, Value: pattern.Format})
	}
	if pattern.Symbol != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "symbol"}, Value: pattern.Symbol})
	}
	patternStart.Attr = attrs

	if err := enc.EncodeToken(patternStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(patternStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeTimePattern(enc *xml.Encoder, pattern *types.XFATimePattern) error {
	patternStart := xml.StartElement{Name: xml.Name{Local: "timePattern"}}
	attrs := []xml.Attr{}
	if pattern.Format != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "format"}, Value: pattern.Format})
	}
	if pattern.Symbol != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "symbol"}, Value: pattern.Symbol})
	}
	patternStart.Attr = attrs

	if err := enc.EncodeToken(patternStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(patternStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeNumberPattern(enc *xml.Encoder, pattern *types.XFANumberPattern) error {
	patternStart := xml.StartElement{Name: xml.Name{Local: "numberPattern"}}
	attrs := []xml.Attr{}
	if pattern.Format != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "format"}, Value: pattern.Format})
	}
	if pattern.Symbol != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "symbol"}, Value: pattern.Symbol})
	}
	if pattern.Grouping != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "grouping"}, Value: pattern.Grouping})
	}
	if pattern.Decimal != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "decimal"}, Value: pattern.Decimal})
	}
	if pattern.Precision > 0 {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "precision"}, Value: strconv.Itoa(pattern.Precision)})
	}
	patternStart.Attr = attrs

	if err := enc.EncodeToken(patternStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(patternStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeText(enc *xml.Encoder, text *types.XFAText) error {
	textStart := xml.StartElement{Name: xml.Name{Local: "text"}}
	attrs := []xml.Attr{}
	if text.Direction != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "direction"}, Value: text.Direction})
	}
	if text.Encoding != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "encoding"}, Value: text.Encoding})
	}
	textStart.Attr = attrs

	if err := enc.EncodeToken(textStart); err != nil {
		return err
	}
	if err := enc.EncodeToken(textStart.End()); err != nil {
		return err
	}

	return nil
}
