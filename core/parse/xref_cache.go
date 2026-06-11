package parse

import (
	"fmt"
	"sync"
)

// xrefCacheKey identifies a pdfBytes slice by backing-array pointer and
// length. Code in this repository treats PDF bytes as immutable — every
// modification path (fill, decrypt, incremental update) allocates a new
// buffer — so an identical (pointer, length) pair means identical bytes.
// Holding the *byte also keeps the backing array alive, so a freed buffer's
// address cannot be recycled for a different PDF while its entry is cached.
type xrefCacheKey struct {
	data *byte
	size int
}

type xrefCacheEntry struct {
	key    xrefCacheKey
	result *XRefResult
	err    error
}

// xrefCacheSize bounds retained memory to a handful of documents.
const xrefCacheSize = 4

var (
	xrefCacheMu sync.Mutex
	xrefCache   []*xrefCacheEntry
)

// mergedXRefChain returns the merged cross-reference chain for pdfBytes
// (last startxref, /Prev links followed, newest entry wins), memoized so
// per-object lookups — GetObject is called once per field in form-parsing
// loops — don't re-walk and re-decompress every xref stream in the chain on
// each call. Parse failures are cached too, so files the chain walker cannot
// handle fall through to their legacy path without paying for a re-walk.
//
// The returned XRefResult is shared between callers and must not be mutated.
func mergedXRefChain(pdfBytes []byte, verbose bool) (*XRefResult, error) {
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("empty PDF")
	}
	key := xrefCacheKey{data: &pdfBytes[0], size: len(pdfBytes)}

	xrefCacheMu.Lock()
	for i, e := range xrefCache {
		if e.key == key {
			// Move to front (LRU).
			copy(xrefCache[1:i+1], xrefCache[:i])
			xrefCache[0] = e
			xrefCacheMu.Unlock()
			return e.result, e.err
		}
	}
	xrefCacheMu.Unlock()

	// Parse outside the lock so a slow chain walk on one document doesn't
	// block cached lookups for others.
	incParser := newIncrementalParser(pdfBytes, verbose)
	entry := &xrefCacheEntry{key: key}
	if entry.err = incParser.parse(); entry.err == nil {
		entry.result = incParser.getFullXRefResult()
	}

	xrefCacheMu.Lock()
	defer xrefCacheMu.Unlock()
	// A concurrent caller may have inserted the same key while we parsed;
	// prefer the existing entry so duplicates don't waste cache slots.
	for i, e := range xrefCache {
		if e.key == key {
			copy(xrefCache[1:i+1], xrefCache[:i])
			xrefCache[0] = e
			return e.result, e.err
		}
	}
	if len(xrefCache) < xrefCacheSize {
		xrefCache = append(xrefCache, nil)
	}
	copy(xrefCache[1:], xrefCache)
	xrefCache[0] = entry
	return entry.result, entry.err
}
