package write

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// PDFAConformance specifies the PDF/A conformance level.
type PDFAConformance int

const (
	PDFA1b PDFAConformance = iota + 1 // ISO 19005-1, conformance B
	PDFA2b                            // ISO 19005-2, conformance B
	PDFA3b                            // ISO 19005-3, conformance B
)

// PDFAOptions configures a PDF/A document.
type PDFAOptions struct {
	Conformance  PDFAConformance // defaults to PDFA2b
	Title        string
	Author       string
	Subject      string
	Keywords     string
	Creator      string    // originating tool; defaults to "pdfer"
	CreationDate time.Time // zero → time of Bytes() call
}

// preparePDFA writes an XMP metadata stream, an sRGB ICC profile stream, and
// an OutputIntent object into w. It returns the object numbers of the metadata
// stream and the OutputIntent so the caller can reference them from the catalog.
// It also sets the PDF version to match the conformance level.
func (w *PDFWriter) preparePDFA(opts PDFAOptions, now time.Time) (metaObjNum, outputIntentObjNum int, err error) {
	if opts.Conformance == 0 {
		opts.Conformance = PDFA2b
	}

	var part int
	switch opts.Conformance {
	case PDFA1b:
		part = 1
		w.SetVersion("1.4")
	case PDFA3b:
		part = 3
		w.SetVersion("1.7")
	default:
		part = 2
		w.SetVersion("1.7")
	}

	// PDF/A requires a trailer /ID array (ISO 19005 §6.7.3). Generate one if
	// the writer doesn't already have a file ID (i.e. no encryption was set up).
	if len(w.fileID) == 0 {
		id := make([]byte, 16)
		if _, randErr := rand.Read(id); randErr == nil {
			w.fileID = id
		}
	}

	created := opts.CreationDate
	if created.IsZero() {
		created = now
	}

	xmpData := buildXMPMetadata(opts, part, created, now)
	// XMP metadata must not be compressed (PDF/A requirement).
	metaObjNum = w.AddStreamObject(Dictionary{
		"Type":    "/Metadata",
		"Subtype": "/XML",
	}, xmpData, false)

	iccData := buildSRGBICCProfile()
	iccObjNum := w.AddStreamObject(Dictionary{
		"N": 3,
	}, iccData, true)

	outputIntentObjNum = w.AddObject([]byte(fmt.Sprintf(
		"<</Type/OutputIntent/S/GTS_PDFA1/OutputConditionIdentifier(sRGB IEC61966-2.1)/Info(sRGB IEC61966-2.1)/DestOutputProfile %d 0 R>>",
		iccObjNum,
	)))

	return metaObjNum, outputIntentObjNum, nil
}

func buildXMPMetadata(opts PDFAOptions, part int, created, modified time.Time) []byte {
	dateStr := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }
	creator := opts.Creator
	if creator == "" {
		creator = "pdfer"
	}

	var b bytes.Buffer
	// xpacket begin must contain the UTF-8 BOM (U+FEFF = 0xEF 0xBB 0xBF).
	b.WriteString("<?xpacket begin=\"\xEF\xBB\xBF\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n")
	b.WriteString("  <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	b.WriteString("    <rdf:Description rdf:about=\"\"\n")
	b.WriteString("        xmlns:dc=\"http://purl.org/dc/elements/1.1/\"\n")
	b.WriteString("        xmlns:xmp=\"http://ns.adobe.com/xap/1.0/\"\n")
	b.WriteString("        xmlns:pdf=\"http://ns.adobe.com/pdf/1.3/\"\n")
	b.WriteString("        xmlns:pdfaid=\"http://www.aiim.org/pdfa/ns/id/\">\n")
	b.WriteString("      <dc:format>application/pdf</dc:format>\n")
	if opts.Title != "" {
		fmt.Fprintf(&b, "      <dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">%s</rdf:li></rdf:Alt></dc:title>\n", xmlEscape(opts.Title))
	}
	if opts.Author != "" {
		fmt.Fprintf(&b, "      <dc:creator><rdf:Seq><rdf:li>%s</rdf:li></rdf:Seq></dc:creator>\n", xmlEscape(opts.Author))
	}
	if opts.Subject != "" {
		fmt.Fprintf(&b, "      <dc:description><rdf:Alt><rdf:li xml:lang=\"x-default\">%s</rdf:li></rdf:Alt></dc:description>\n", xmlEscape(opts.Subject))
	}
	fmt.Fprintf(&b, "      <xmp:CreateDate>%s</xmp:CreateDate>\n", dateStr(created))
	fmt.Fprintf(&b, "      <xmp:ModifyDate>%s</xmp:ModifyDate>\n", dateStr(modified))
	fmt.Fprintf(&b, "      <xmp:CreatorTool>%s</xmp:CreatorTool>\n", xmlEscape(creator))
	b.WriteString("      <pdf:Producer>pdfer</pdf:Producer>\n")
	if opts.Keywords != "" {
		fmt.Fprintf(&b, "      <pdf:Keywords>%s</pdf:Keywords>\n", xmlEscape(opts.Keywords))
	}
	fmt.Fprintf(&b, "      <pdfaid:part>%d</pdfaid:part>\n", part)
	b.WriteString("      <pdfaid:conformance>B</pdfaid:conformance>\n")
	b.WriteString("    </rdf:Description>\n")
	b.WriteString("  </rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")
	return b.Bytes()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// buildSRGBICCProfile constructs a minimal ICC v2 profile for sRGB (IEC 61966-2.1).
// The profile contains 9 tags: desc, cprt, wtpt, rXYZ, gXYZ, bXYZ, rTRC, gTRC, bTRC.
// The three TRC tags share a single gamma=2.2 curve.
func buildSRGBICCProfile() []byte {
	// Layout (all sizes in bytes, offsets relative to file start):
	//   0-127   header (128)
	//   128-131 tag count (4)
	//   132-239 tag table: 9 × 12 bytes (108)
	//   240-347 desc tag (108)
	//   348-399 cprt tag (52)
	//   400-419 wtpt tag (20)
	//   420-439 rXYZ tag (20)
	//   440-459 gXYZ tag (20)
	//   460-479 bXYZ tag (20)
	//   480-495 rTRC/gTRC/bTRC shared tag (16)
	//   total = 496 bytes
	const (
		headerSize = 128
		tagCount   = 9
		dataStart  = headerSize + 4 + tagCount*12 // 240

		descOffset  = dataStart // 240
		descSize    = 108
		cprtOffset  = descOffset + descSize // 348
		cprtSize    = 52
		wtptOffset  = cprtOffset + cprtSize // 400
		xyzSize     = 20
		rXYZOffset  = wtptOffset + xyzSize // 420
		gXYZOffset  = rXYZOffset + xyzSize // 440
		bXYZOffset  = gXYZOffset + xyzSize // 460
		trcOffset   = bXYZOffset + xyzSize // 480
		trcSize     = 16
		profileSize = trcOffset + trcSize // 496
	)

	profile := make([]byte, profileSize)

	put4 := func(b []byte, v uint32) { binary.BigEndian.PutUint32(b, v) }
	put2 := func(b []byte, v uint16) { binary.BigEndian.PutUint16(b, v) }
	// s15Fixed16Number: integer.fraction where each part is 16 bits.
	// Encoding: round(value × 65536) stored as big-endian uint32.
	s15f16 := func(v float64) uint32 { return uint32(int32(v*65536 + 0.5)) }

	// Header
	put4(profile[0:], profileSize)
	put4(profile[8:], 0x02100000)                  // ICC version 2.1.0.0
	copy(profile[12:], "mntr")                     // profile class: monitor device
	copy(profile[16:], "RGB ")                     // data color space
	copy(profile[20:], "XYZ ")                     // Profile Connection Space
	binary.BigEndian.PutUint16(profile[24:], 2000) // creation year
	binary.BigEndian.PutUint16(profile[26:], 1)    // month
	binary.BigEndian.PutUint16(profile[28:], 1)    // day
	copy(profile[36:], "acsp")                     // profile file signature
	copy(profile[40:], "MSFT")                     // primary platform
	// Bytes 44-67: flags, manufacturer, model, attributes, rendering intent = 0
	// PCS illuminant: D50
	put4(profile[68:], s15f16(0.9642))
	put4(profile[72:], s15f16(1.0000))
	put4(profile[76:], s15f16(0.8249))
	// Bytes 80-127: creator, profile ID, reserved = 0

	// Tag count
	put4(profile[128:], tagCount)

	// Tag table (9 entries of 12 bytes each: signature[4], offset[4], size[4])
	type entry struct {
		sig     string
		off, sz uint32
	}
	table := []entry{
		{"desc", descOffset, descSize},
		{"cprt", cprtOffset, cprtSize},
		{"wtpt", wtptOffset, xyzSize},
		{"rXYZ", rXYZOffset, xyzSize},
		{"gXYZ", gXYZOffset, xyzSize},
		{"bXYZ", bXYZOffset, xyzSize},
		{"rTRC", trcOffset, trcSize},
		{"gTRC", trcOffset, trcSize}, // shared data with rTRC
		{"bTRC", trcOffset, trcSize}, // shared data with rTRC
	}
	for i, e := range table {
		te := profile[132+i*12:]
		copy(te[0:4], e.sig)
		put4(te[4:], e.off)
		put4(te[8:], e.sz)
	}

	// desc tag: ICC v2 "desc" type
	// Structure: type(4), reserved(4), ASCII count(4), ASCII string, Unicode(8), Mac(70)
	dt := profile[descOffset:]
	copy(dt[0:4], "desc")
	descStr := "sRGB IEC61966-2.1"
	put4(dt[8:], uint32(len(descStr)+1)) // count includes null terminator
	copy(dt[12:], descStr)
	// Unicode lang (4), Unicode count (4), Mac script (2), Mac count (1), Mac string (67) = all 0

	// cprt tag: ICC v2 "text" type
	ct := profile[cprtOffset:]
	copy(ct[0:4], "text")
	copy(ct[8:], "Copyright (C) 1999 Hewlett-Packard Company")
	// Null terminator and padding remain 0.

	// XYZ tags: ICC v2 "XYZ " type (type(4) + reserved(4) + XYZ(12) = 20 bytes)
	writeXYZ := func(off int, x, y, z float64) {
		xt := profile[off:]
		copy(xt[0:4], "XYZ ")
		put4(xt[8:], s15f16(x))
		put4(xt[12:], s15f16(y))
		put4(xt[16:], s15f16(z))
	}
	writeXYZ(wtptOffset, 0.9642, 1.0000, 0.8249) // D50 white point
	// sRGB primary colorants, Bradford-adapted to D50:
	writeXYZ(rXYZOffset, 0.4361, 0.2225, 0.0139)
	writeXYZ(gXYZOffset, 0.3851, 0.7169, 0.0971)
	writeXYZ(bXYZOffset, 0.1431, 0.0606, 0.7142)

	// TRC tags (all three share this data): ICC v2 "curv" type with count=1.
	// A single-entry curv encodes a pure gamma: value/256 gives the exponent.
	// 563/256 ≈ 2.199 ≈ 2.2 (sRGB approximate gamma).
	tt := profile[trcOffset:]
	copy(tt[0:4], "curv")
	put4(tt[8:], 1)    // count = 1 → single gamma value
	put2(tt[12:], 563) // 563/256 ≈ 2.2

	return profile
}
