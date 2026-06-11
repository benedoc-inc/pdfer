// Command pdfer is a BeneDoc-internal tool for filling XFA forms, extracting
// XFA schemas from eSTAR PDFs, and stamping/watermarking PDFs.
// It is not a general-purpose PDF CLI — for the full library API see the root
// pdfer package documentation.
//
// Subcommands:
//
//	pdfer stamp -input a.pdf -output b.pdf [-text DRAFT] [-opacity 0.12] [-angle 45] [-font-size 80] [-x 420] [-y 760]
//	pdfer (no subcommand) — XFA form filling (legacy behaviour)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	encrypt "github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/forms/xfa"
	"github.com/benedoc-inc/pdfer/v2/types"
)

func main() {
	// Catch panics and write to stderr
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			os.Exit(1)
		}
	}()

	// Dispatch subcommands before the legacy flag.Parse() path.
	if len(os.Args) > 1 && os.Args[1] == "stamp" {
		handleStamp(os.Args[2:])
		return
	}

	var (
		inputPDF      = flag.String("input", "", "Path to input eSTAR PDF file")
		dataJSON      = flag.String("data", "", "Path to JSON file with form data")
		outputPDF     = flag.String("output", "", "Path to output filled PDF file")
		verbose       = flag.Bool("verbose", false, "Enable verbose logging")
		logFile       = flag.String("log", "", "Path to log file (if empty, logs to stderr)")
		verify        = flag.Bool("verify", false, "Run verification test with UniPDF instead of filling form")
		extractSchema = flag.Bool("extract-schema", false, "Extract questionnaire schema from PDF and output as JSON (requires -output)")
		listAttach    = flag.Bool("list-attachments", false, "List embedded file attachments in the PDF and exit")
		extractAttach = flag.String("extract-attachments", "", "Extract embedded file attachments into the given directory and exit")
	)
	flag.Parse()

	// Force stderr to be unbuffered
	os.Stderr.WriteString("=== pdfer starting ===\n")

	// If verify flag is set, run verification test
	if *verify {
		if *inputPDF == "" {
			log.Fatal("Error: -input flag is required for verification")
		}
		log.Fatal("Error: -verify flag is not supported (verify_unipdf.go has been removed)")
	}

	if *inputPDF == "" {
		log.Fatal("Error: -input flag is required")
	}

	// If extracting schema, handle that separately
	if *extractSchema {
		if *outputPDF == "" {
			log.Fatal("Error: -output flag is required when using -extract-schema")
		}
		handleExtractSchema(*inputPDF, *outputPDF, *verbose)
		return
	}

	// Attachment listing / extraction also short-circuit the fill path.
	if *listAttach {
		handleListAttachments(*inputPDF)
		return
	}
	if *extractAttach != "" {
		handleExtractAttachments(*inputPDF, *extractAttach)
		return
	}

	if *dataJSON == "" {
		log.Fatal("Error: -data flag is required")
	}
	if *outputPDF == "" {
		log.Fatal("Error: -output flag is required")
	}

	// Set up logging - write to both file and stderr if log file specified
	var logF *os.File
	if *logFile != "" {
		var err error
		logF, err = os.Create(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating log file: %v\n", err)
			os.Exit(1)
		}
		// Write to both file and stderr using a multi-writer
		log.SetOutput(logF)
		fmt.Fprintf(os.Stderr, "Logging to: %s\n", *logFile)
		fmt.Fprintf(logF, "=== pdfer started ===\n")
		logF.Sync()
	} else {
		log.SetOutput(os.Stderr)
	}

	// Ensure log file is closed at end
	if logF != nil {
		defer func() {
			fmt.Fprintf(logF, "=== pdfer finished ===\n")
			logF.Sync()
			logF.Close()
		}()
	}

	if *verbose {
		log.Printf("Input PDF: %s", *inputPDF)
		log.Printf("Data JSON: %s", *dataJSON)
		log.Printf("Output PDF: %s", *outputPDF)
		if *logFile != "" {
			log.Printf("Log file: %s", *logFile)
		}
	}

	// Read form data JSON
	dataBytes, err := os.ReadFile(*dataJSON)
	if err != nil {
		log.Fatalf("Error reading data file: %v", err)
	}

	var formData types.FormData
	err = json.Unmarshal(dataBytes, &formData)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	if *verbose {
		log.Printf("Loaded %d field values from JSON", len(formData))
	}

	// Read PDF file
	pdfBytes, err := os.ReadFile(*inputPDF)
	if err != nil {
		log.Fatalf("Error reading PDF: %v", err)
	}

	// Extract the form via the unified library entry point. This handles
	// encryption (decrypts and rewrites the PDF to classical-xref plaintext)
	// before fill, which is what makes XFA fill work on PDF 1.5+ inputs that
	// use cross-reference streams — the original CLI path called
	// xfa.UpdateXFAInPDF directly on the encrypted bytes and silently
	// corrupted the xref on every eSTAR fixture (issue #12 will revisit
	// whether to preserve encryption via incremental updates).
	form, err := extractFormTryingCommonPasswords(pdfBytes, *verbose)
	if err != nil {
		log.Fatalf("Could not extract form: %v", err)
	}

	updatedPDF, err := form.Fill(pdfBytes, formData, nil, *verbose)
	if err != nil {
		log.Fatalf("Error filling form: %v", err)
	}

	// Write updated PDF
	err = os.WriteFile(*outputPDF, updatedPDF, 0644)
	if err != nil {
		log.Fatalf("Error writing PDF: %v", err)
	}

	// Write success message to log file if it exists
	if logF != nil {
		fmt.Fprintf(logF, "Successfully filled form\n")
		fmt.Fprintf(logF, "Input:  %s\n", *inputPDF)
		fmt.Fprintf(logF, "Data:   %s\n", *dataJSON)
		fmt.Fprintf(logF, "Output: %s\n", *outputPDF)
		fmt.Fprintf(logF, "Fields filled: %d\n", len(formData))
		logF.Sync()
	}

	// Always print to stderr so it's visible
	fmt.Fprintf(os.Stderr, "Successfully filled form\n")
	fmt.Fprintf(os.Stderr, "Input:  %s\n", *inputPDF)
	fmt.Fprintf(os.Stderr, "Data:   %s\n", *dataJSON)
	fmt.Fprintf(os.Stderr, "Output: %s\n", *outputPDF)
	fmt.Fprintf(os.Stderr, "Fields filled: %d\n", len(formData))

	// Also print to stdout
	fmt.Printf("Successfully filled form\n")
	fmt.Printf("Input:  %s\n", *inputPDF)
	fmt.Printf("Data:   %s\n", *dataJSON)
	fmt.Printf("Output: %s\n", *outputPDF)
	fmt.Printf("Fields filled: %d\n", len(formData))
}

// handleStamp adds a text watermark to every page of a PDF.
func handleStamp(args []string) {
	fs := flag.NewFlagSet("stamp", flag.ExitOnError)
	input := fs.String("input", "", "Path to input PDF (required)")
	output := fs.String("output", "", "Path to output PDF (defaults to input, overwriting in place)")
	text := fs.String("text", "CONFIDENTIAL DRAFT", "Stamp text")
	opacity := fs.Float64("opacity", 0.55, "Opacity 0.0–1.0")
	fontSize := fs.Float64("font-size", 9.0, "Font size in points")
	angle := fs.Float64("angle", 0, "Rotation in degrees, counter-clockwise")
	x := fs.Float64("x", 420, "Stamp X position in points from page origin")
	y := fs.Float64("y", 760, "Stamp Y position in points from page origin")
	fs.Parse(args)

	if *input == "" {
		fmt.Fprintln(os.Stderr, "Error: -input is required")
		fs.Usage()
		os.Exit(1)
	}
	dest := *output
	if dest == "" {
		dest = *input
	}

	pdfBytes, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", *input, err)
		os.Exit(1)
	}

	stamp := pdfer.TextStamp{
		Text:     *text,
		FontName: "Helvetica-Bold",
		FontSize: *fontSize,
		X:        *x,
		Y:        *y,
		Opacity:  *opacity,
		Angle:    *angle,
		R:        0.55,
		G:        0.55,
		B:        0.55,
	}

	out, err := pdfer.StampAllPages(pdfBytes, stamp, nil, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error stamping PDF: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(dest, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dest, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Stamped %d bytes → %s\n", len(out), dest)
}

// extractFormTryingCommonPasswords runs pdfer.ExtractForm with the empty
// password first (the eSTAR default), then a small list of common
// developer-facing fallbacks before giving up.
func extractFormTryingCommonPasswords(pdfBytes []byte, verbose bool) (pdfer.Form, error) {
	if !bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		return pdfer.ExtractForm(pdfBytes, nil, verbose)
	}
	if verbose {
		log.Printf("PDF is encrypted, trying empty password then common fallbacks...")
	}
	passwords := [][]byte{[]byte(""), []byte("admin"), []byte("password"), []byte("1234")}
	var lastErr error
	for _, pwd := range passwords {
		form, err := pdfer.ExtractForm(pdfBytes, pwd, verbose)
		if err == nil {
			if verbose {
				if len(pwd) == 0 {
					log.Printf("Decrypted with empty password")
				} else {
					log.Printf("Decrypted with fallback password %q", string(pwd))
				}
			}
			return form, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("could not decrypt PDF with any common password: %w", lastErr)
}

// handleExtractSchema extracts questionnaire schema from PDF and writes it as JSON
func handleExtractSchema(inputPDF, outputJSON string, verbose bool) {
	// Read PDF file
	pdfBytes, err := os.ReadFile(inputPDF)
	if err != nil {
		log.Fatalf("Error reading PDF: %v", err)
	}

	// Check if PDF is encrypted and decrypt if needed
	var encryptInfo *types.PDFEncryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		if verbose {
			log.Printf("PDF is encrypted, attempting to decrypt...")
		}

		// Try to decrypt with empty password (most eSTAR PDFs allow this)
		encInfo, err := encrypt.DecryptPDF(pdfBytes, []byte(""), verbose)
		if err != nil {
			if verbose {
				log.Printf("Empty password failed, trying common passwords...")
			}
			// Try common passwords
			commonPasswords := [][]byte{[]byte(""), []byte("admin"), []byte("password"), []byte("1234")}
			decrypted := false
			for _, pwd := range commonPasswords {
				tryInfo, tryErr := encrypt.DecryptPDF(pdfBytes, pwd, verbose)
				if tryErr == nil {
					encInfo = tryInfo
					encryptInfo = encInfo
					decrypted = true
					if verbose {
						log.Printf("Successfully decrypted PDF")
					}
					break
				}
			}
			if !decrypted {
				log.Fatalf("Could not decrypt PDF: %v", err)
			}
		} else {
			encryptInfo = encInfo
			if verbose {
				log.Printf("Successfully decrypted PDF with empty password")
			}
		}
	}

	// Extract XFA data from PDF
	xfaData, _, err := xfa.FindXFADatasetsStream(pdfBytes, encryptInfo, verbose)
	if err != nil {
		log.Fatalf("Error finding XFA datasets stream: %v", err)
	}

	// Decompress XFA XML
	xfaXML, _, err := xfa.DecompressStream(xfaData)
	if err != nil {
		log.Fatalf("Error decompressing XFA stream: %v", err)
	}

	// Parse XFA to FormSchema
	schema, err := xfa.ParseXFAForm(string(xfaXML), verbose)
	if err != nil {
		log.Fatalf("Error parsing XFA form: %v", err)
	}

	// Write schema as JSON
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling schema to JSON: %v", err)
	}

	err = os.WriteFile(outputJSON, schemaJSON, 0644)
	if err != nil {
		log.Fatalf("Error writing schema JSON: %v", err)
	}

	// Write success message
	fmt.Fprintf(os.Stderr, "Successfully extracted questionnaire schema\n")
	fmt.Fprintf(os.Stderr, "Input:  %s\n", inputPDF)
	fmt.Fprintf(os.Stderr, "Output: %s\n", outputJSON)
	fmt.Fprintf(os.Stderr, "Questions extracted: %d\n", len(schema.Questions))

	fmt.Printf("Successfully extracted questionnaire schema\n")
	fmt.Printf("Input:  %s\n", inputPDF)
	fmt.Printf("Output: %s\n", outputJSON)
	fmt.Printf("Questions extracted: %d\n", len(schema.Questions))
}

// handleListAttachments prints the embedded file attachments found in the PDF.
func handleListAttachments(inputPDF string) {
	pdfBytes, err := os.ReadFile(inputPDF)
	if err != nil {
		log.Fatalf("Error reading PDF: %v", err)
	}
	attachments, err := pdfer.ListAttachments(pdfBytes)
	if err != nil {
		log.Fatalf("Error listing attachments: %v", err)
	}
	if len(attachments) == 0 {
		fmt.Println("No embedded file attachments found.")
		return
	}
	fmt.Printf("Found %d attachment(s):\n", len(attachments))
	for _, a := range attachments {
		mime := a.MimeType
		if mime == "" {
			mime = "(unknown)"
		}
		fmt.Printf("  %-40s %10d bytes  %s\n", a.Name, len(a.Data), mime)
	}
}

// handleExtractAttachments writes every embedded file attachment into outDir.
func handleExtractAttachments(inputPDF, outDir string) {
	pdfBytes, err := os.ReadFile(inputPDF)
	if err != nil {
		log.Fatalf("Error reading PDF: %v", err)
	}
	attachments, err := pdfer.ListAttachments(pdfBytes)
	if err != nil {
		log.Fatalf("Error listing attachments: %v", err)
	}
	if len(attachments) == 0 {
		fmt.Println("No embedded file attachments found.")
		return
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}
	// Track names already written so colliding base names don't clobber each
	// other: distinct attachments can share a display name, and filepath.Base
	// can collapse distinct paths to the same name.
	used := make(map[string]bool)
	for _, a := range attachments {
		// Guard against path traversal from attacker-controlled filenames.
		name := filepath.Base(filepath.FromSlash(a.Name))
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = "attachment"
		}
		name = uniqueAttachmentName(name, used)
		dest := filepath.Join(outDir, name)
		if err := os.WriteFile(dest, a.Data, 0644); err != nil {
			log.Fatalf("Error writing %s: %v", dest, err)
		}
		fmt.Printf("Extracted %s (%d bytes)\n", dest, len(a.Data))
	}
}

// uniqueAttachmentName returns name if unused, otherwise inserts a -1, -2, …
// suffix before the extension until the result is unique. The chosen name is
// recorded in used.
func uniqueAttachmentName(name string, used map[string]bool) string {
	if !used[name] {
		used[name] = true
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}
