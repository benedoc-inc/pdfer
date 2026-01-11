# PDF Comparison

The `compare` package provides comprehensive PDF comparison and diffing capabilities.

## Features

### ✅ Implemented

- **Content Comparison**: Compare text, graphics, images, annotations between PDFs
- **Metadata Comparison**: Compare document metadata (title, author, dates, etc.)
- **Structure Comparison**: Compare page counts, document structure
- **Page-by-Page Diff**: Detailed differences per page
- **JSON Reports**: Machine-readable comparison results
- **Human-Readable Reports**: Text-based diff reports
- **Configurable Options**: Ignore metadata fields, adjust tolerance levels

### 🔄 Future Enhancements

The following features would enhance comparison but may require other components:

1. **Visual Diffing** (requires annotation writing):
   - Generate PDFs with differences highlighted
   - Overlay annotations showing changes
   - Side-by-side comparison views

2. **Form Comparison** (requires form extraction integration):
   - Compare AcroForm field values
   - Compare XFA form data
   - Track form field changes

3. **Object-Level Comparison**:
   - Compare raw PDF objects
   - Track object number changes
   - Detect structural modifications

4. **Advanced Matching**:
   - Fuzzy text matching (handle OCR differences)
   - Shape matching for graphics
   - Image similarity comparison

5. **Change Tracking**:
   - Track changes across multiple versions
   - Generate change logs
   - Version history

## Usage

See main README.md for examples.
