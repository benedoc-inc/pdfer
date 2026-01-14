package write

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strconv"
	"strings"
)

// objectStreamGroup represents a group of objects to be stored in an object stream
type objectStreamGroup struct {
	objNums      []int
	streamObjNum int
}

// createObjectStreams groups objects into object streams
// Returns map of object number -> (streamObjNum, indexInStream)
func (w *PDFWriter) createObjectStreams() (map[int]struct {
	streamObjNum int
	index        int
}, []objectStreamGroup, error) {
	if !w.useObjectStream {
		return nil, nil, nil
	}

	// Group non-stream objects into object streams
	// For simplicity, put all eligible objects into one stream (can be optimized later)
	var eligibleObjs []int
	for objNum, obj := range w.objects {
		if obj.IsFree {
			continue
		}
		// Only compress non-stream objects (stream objects are already compressed)
		if obj.Stream == nil && obj.Content != nil {
			eligibleObjs = append(eligibleObjs, objNum)
		}
	}

	if len(eligibleObjs) == 0 {
		return nil, nil, nil
	}

	// Create object stream
	streamObjNum := w.nextObjNum
	w.nextObjNum++

	// Build header and collect object data
	var headerBuilder strings.Builder
	objMap := make(map[int]struct {
		streamObjNum int
		index        int
	})

	// Build header first (we need to know offsets)
	type objInfo struct {
		objNum int
		offset int
		data   []byte
	}
	var objInfos []objInfo

	for _, objNum := range eligibleObjs {
		obj := w.objects[objNum]
		objInfos = append(objInfos, objInfo{
			objNum: objNum,
			data:   obj.Content,
		})
	}

	// Calculate offsets
	currentOffset := 0
	for i := range objInfos {
		objInfos[i].offset = currentOffset
		// Offset is relative to start of object data (after header)
		// We'll add header length later
		currentOffset += len(objInfos[i].data) + 1 // +1 for space separator
	}

	// Build header: "objnum1 offset1 objnum2 offset2 ..."
	for _, info := range objInfos {
		headerBuilder.WriteString(strconv.Itoa(info.objNum))
		headerBuilder.WriteString(" ")
		headerBuilder.WriteString(strconv.Itoa(info.offset))
		headerBuilder.WriteString(" ")
	}

	headerStr := strings.TrimSpace(headerBuilder.String())
	headerBytes := []byte(headerStr)
	firstOffset := len(headerBytes)

	// Adjust offsets to be relative to start of data (after header + space)
	dataStartOffset := firstOffset + 1 // +1 for space after header
	for i := range objInfos {
		objInfos[i].offset += dataStartOffset
	}

	// Rebuild header with corrected offsets
	headerBuilder.Reset()
	for _, info := range objInfos {
		headerBuilder.WriteString(strconv.Itoa(info.objNum))
		headerBuilder.WriteString(" ")
		headerBuilder.WriteString(strconv.Itoa(info.offset))
		headerBuilder.WriteString(" ")
	}
	headerStr = strings.TrimSpace(headerBuilder.String())
	headerBytes = []byte(headerStr)
	firstOffset = len(headerBytes)

	// Build data section
	var dataBuilder bytes.Buffer
	for i, info := range objInfos {
		dataBuilder.Write(info.data)
		if i < len(objInfos)-1 {
			dataBuilder.WriteByte(' ') // Space between objects
		}

		// Track mapping
		objMap[info.objNum] = struct {
			streamObjNum int
			index        int
		}{streamObjNum: streamObjNum, index: i}
	}

	// Combine header and data
	var streamData bytes.Buffer
	streamData.Write(headerBytes)
	streamData.WriteByte(' ') // Space between header and data
	streamData.Write(dataBuilder.Bytes())

	// Compress stream data
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(streamData.Bytes()); err != nil {
		return nil, nil, fmt.Errorf("failed to compress object stream: %v", err)
	}
	if err := zw.Close(); err != nil {
		return nil, nil, fmt.Errorf("failed to close zlib writer: %v", err)
	}
	compressedData := compressed.Bytes()

	// Create object stream dictionary
	objStreamDict := Dictionary{
		"/Type":   "/ObjStm",
		"/N":      len(eligibleObjs),
		"/First":  firstOffset,
		"/Filter": "/FlateDecode",
		"/Length": len(compressedData),
	}

	// Create object stream object
	w.objects[streamObjNum] = &PDFObject{
		Number:     streamObjNum,
		Generation: 0,
		Dict:       objStreamDict,
		Stream:     compressedData,
	}

	group := objectStreamGroup{
		objNums:      eligibleObjs,
		streamObjNum: streamObjNum,
	}

	return objMap, []objectStreamGroup{group}, nil
}
