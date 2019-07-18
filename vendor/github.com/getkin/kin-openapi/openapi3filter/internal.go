package openapi3filter

import (
	"strings"
)

func parseMediaType(contentType string) string {
	i := strings.IndexByte(contentType, ';')
	if i < 0 {
		return contentType
	}
	return contentType[:i]
}
