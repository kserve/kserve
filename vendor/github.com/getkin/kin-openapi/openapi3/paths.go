package openapi3

import (
	"context"
	"fmt"
	"strings"
)

// Paths is specified by OpenAPI/Swagger standard version 3.0.
type Paths map[string]*PathItem

func (paths Paths) Validate(c context.Context) error {
	normalizedPaths := make(map[string]string)
	for path, pathItem := range paths {
		normalizedPath := normalizePathKey(path)
		if oldPath, exists := normalizedPaths[normalizedPath]; exists {
			return fmt.Errorf("Conflicting paths '%v' and '%v'", path, oldPath)
		}
		if path == "" || path[0] != '/' {
			return fmt.Errorf("Path '%v' does not start with '/'", path)
		}
		if strings.Contains(path, "//") {
			return fmt.Errorf("Path '%v' contains '//'", path)
		}
		normalizedPaths[path] = path
		if err := pathItem.Validate(c); err != nil {
			return err
		}
	}
	return nil
}

// Find returns a path that matches the key.
//
// The method ignores differences in template variable names (except possible "*" suffix).
//
// For example:
//
//   paths := openapi3.Paths {
//     "/person/{personName}": &openapi3.PathItem{},
//   }
//   pathItem := path.Find("/person/{name}")
//
// would return the correct path item.
func (paths Paths) Find(key string) *PathItem {
	// Try directly access the map
	pathItem := paths[key]
	if pathItem != nil {
		return pathItem
	}

	// Use normalized keys
	normalizedSearchedPath := normalizePathKey(key)
	for path, pathItem := range paths {
		normalizedPath := normalizePathKey(path)
		if normalizedPath == normalizedSearchedPath {
			return pathItem
		}
	}
	return nil
}

func normalizePathKey(key string) string {
	// If the argument has no path variables, return the argument
	if strings.IndexByte(key, '{') < 0 {
		return key
	}

	// Allocate buffer
	buf := make([]byte, 0, len(key))

	// Visit each byte
	isVariable := false
	for i := 0; i < len(key); i++ {
		c := key[i]
		if isVariable {
			if c == '}' {
				// End path variables
				// First append possible '*' before this character
				// The character '}' will be appended
				if i > 0 && key[i-1] == '*' {
					buf = append(buf, '*')
				}
				isVariable = false
			} else {
				// Skip this character
				continue
			}
		} else if c == '{' {
			// Begin path variable
			// The character '{' will be appended
			isVariable = true
		}

		// Append the character
		buf = append(buf, c)
	}
	return string(buf)
}
