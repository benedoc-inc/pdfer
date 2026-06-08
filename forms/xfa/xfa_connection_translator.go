package xfa

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseXFAConnectionSet parses XFA connectionSet XML and converts it to XFAConnectionSet type
func ParseXFAConnectionSet(xfaConnectionSetXML string, verbose bool) (*types.XFAConnectionSet, error) {
	if verbose {
		log.Printf("Parsing XFA connectionSet XML (length: %d bytes)", len(xfaConnectionSetXML))
	}

	connectionSet := &types.XFAConnectionSet{
		Connections: make([]types.XFAConnection, 0),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaConnectionSetXML))
	decoder.Strict = false

	var currentConnection *types.XFAConnection
	var currentValue strings.Builder
	var inDataDescription bool

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
			case "connectionSet":
				// Root element
			case "connection":
				currentConnection = &types.XFAConnection{
					Properties: make(map[string]interface{}),
				}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						currentConnection.Name = attr.Value
					case "type":
						currentConnection.Type = attr.Value
					}
				}
			case "dataDescription":
				if currentConnection != nil {
					inDataDescription = true
					if currentConnection.DataDescription == nil {
						currentConnection.DataDescription = &types.XFADataDescription{
							Properties: make(map[string]interface{}),
						}
					}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "ref":
							currentConnection.DataDescription.Ref = attr.Value
						case "schema":
							currentConnection.DataDescription.Schema = attr.Value
						case "namespace":
							currentConnection.DataDescription.Namespace = attr.Value
						}
					}
				}
			case "uri", "url":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "wsdl":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "operation":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "soapAction":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "user":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "password":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "dsn":
				if currentConnection != nil {
					currentValue.Reset()
				}
			case "query":
				if currentConnection != nil {
					currentValue.Reset()
				}
			default:
				if currentConnection != nil {
					currentValue.Reset()
				}
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "connection":
				if currentConnection != nil {
					connectionSet.Connections = append(connectionSet.Connections, *currentConnection)
					if verbose {
						log.Printf("Parsed connection: %s (type: %s)", currentConnection.Name, currentConnection.Type)
					}
				}
				currentConnection = nil
			case "dataDescription":
				inDataDescription = false
			case "uri", "url":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["uri"] = value
					}
				}
			case "wsdl":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["wsdl"] = value
					}
				}
			case "operation":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["operation"] = value
					}
				}
			case "soapAction":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["soapAction"] = value
					}
				}
			case "user":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["user"] = value
					}
				}
			case "password":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["password"] = value
					}
				}
			case "dsn":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["dsn"] = value
					}
				}
			case "query":
				if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties["query"] = value
					}
				}
			default:
				if inDataDescription && currentConnection != nil && currentConnection.DataDescription != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.DataDescription.Properties[localName] = value
					}
				} else if currentConnection != nil {
					value := strings.TrimSpace(currentValue.String())
					if value != "" {
						currentConnection.Properties[localName] = value
					}
				}
				currentValue.Reset()
			}

		case xml.CharData:
			currentValue.WriteString(string(se))
		}
	}

	if verbose {
		log.Printf("Parsed XFA connectionSet: %d connections", len(connectionSet.Connections))
	}

	return connectionSet, nil
}

// ConnectionSetToXFA converts XFAConnectionSet to XFA XML
func ConnectionSetToXFA(connectionSet *types.XFAConnectionSet, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting XFAConnectionSet to XFA XML: %d connections", len(connectionSet.Connections))
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start connectionSet element
	connectionSetStart := xml.StartElement{Name: xml.Name{Local: "connectionSet"}}
	if err := enc.EncodeToken(connectionSetStart); err != nil {
		return "", fmt.Errorf("failed to encode connectionSet start: %v", err)
	}

	// Encode connections
	for _, connection := range connectionSet.Connections {
		if err := encodeConnection(enc, connection); err != nil {
			return "", fmt.Errorf("failed to encode connection %s: %v", connection.Name, err)
		}
	}

	// End connectionSet element
	if err := enc.EncodeToken(connectionSetStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode connectionSet end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted XFAConnectionSet to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

func encodeConnection(enc *xml.Encoder, connection types.XFAConnection) error {
	connectionStart := xml.StartElement{Name: xml.Name{Local: "connection"}}
	attrs := []xml.Attr{}
	if connection.Name != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "name"}, Value: connection.Name})
	}
	if connection.Type != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "type"}, Value: connection.Type})
	}
	connectionStart.Attr = attrs

	if err := enc.EncodeToken(connectionStart); err != nil {
		return err
	}

	// Encode dataDescription
	if connection.DataDescription != nil {
		if err := encodeDataDescription(enc, connection.DataDescription); err != nil {
			return err
		}
	}

	// Encode properties
	if connection.Properties != nil {
		for key, value := range connection.Properties {
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

	if err := enc.EncodeToken(connectionStart.End()); err != nil {
		return err
	}

	return nil
}

func encodeDataDescription(enc *xml.Encoder, desc *types.XFADataDescription) error {
	descStart := xml.StartElement{Name: xml.Name{Local: "dataDescription"}}
	attrs := []xml.Attr{}
	if desc.Ref != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "ref"}, Value: desc.Ref})
	}
	if desc.Schema != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "schema"}, Value: desc.Schema})
	}
	if desc.Namespace != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "namespace"}, Value: desc.Namespace})
	}
	descStart.Attr = attrs

	if err := enc.EncodeToken(descStart); err != nil {
		return err
	}

	// Encode properties
	if desc.Properties != nil {
		for key, value := range desc.Properties {
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

	if err := enc.EncodeToken(descStart.End()); err != nil {
		return err
	}

	return nil
}
