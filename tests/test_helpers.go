package tests

import (
	"os"
	"path/filepath"
)

// getTestResourcePath returns the path to a test resource file
// This is a shared helper used across all test files
func getTestResourcePath(filename string) string {
	// Try multiple possible locations (works from different working directories)
	possiblePaths := []string{
		filepath.Join("tests", "resources", filename),
		filepath.Join("resources", filename),
		filepath.Join("..", "tests", "resources", filename),
		filepath.Join(".", "tests", "resources", filename),
		filepath.Join("..", "..", "tests", "resources", filename),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Return default path (caller should check if file exists)
	return filepath.Join("tests", "resources", filename)
}

// ensureTestResourceDir ensures the tests/resources directory exists
func ensureTestResourceDir() error {
	dir := filepath.Join("tests", "resources")
	return os.MkdirAll(dir, 0755)
}
