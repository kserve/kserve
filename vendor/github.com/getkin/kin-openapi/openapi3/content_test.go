package openapi3

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContent_Get(t *testing.T) {
	fallback := NewMediaType()
	wildcard := NewMediaType()
	stripped := NewMediaType()
	fullMatch := NewMediaType()
	content := Content{
		"*/*":                             fallback,
		"application/*":                   wildcard,
		"application/json":                stripped,
		"application/json;encoding=utf-8": fullMatch,
	}
	contentWithoutWildcards := Content{
		"application/json":                stripped,
		"application/json;encoding=utf-8": fullMatch,
	}
	tests := []struct {
		name    string
		content Content
		mime    string
		want    *MediaType
	}{
		{
			name:    "missing",
			content: contentWithoutWildcards,
			mime:    "text/plain;encoding=utf-8",
			want:    nil,
		},
		{
			name:    "full match",
			content: content,
			mime:    "application/json;encoding=utf-8",
			want:    fullMatch,
		},
		{
			name:    "stripped match",
			content: content,
			mime:    "application/json;encoding=utf-16",
			want:    stripped,
		},
		{
			name:    "wildcard match",
			content: content,
			mime:    "application/yaml;encoding=utf-16",
			want:    wildcard,
		},
		{
			name:    "fallback match",
			content: content,
			mime:    "text/plain;encoding=utf-16",
			want:    fallback,
		},
		{
			name:    "invalid mime type",
			content: content,
			mime:    "text;encoding=utf16",
			want:    nil,
		},
		{
			name:    "missing no encoding",
			content: contentWithoutWildcards,
			mime:    "text/plain",
			want:    nil,
		},
		{
			name:    "stripped match no encoding",
			content: content,
			mime:    "application/json",
			want:    stripped,
		},
		{
			name:    "wildcard match no encoding",
			content: content,
			mime:    "application/yaml",
			want:    wildcard,
		},
		{
			name:    "fallback match no encoding",
			content: content,
			mime:    "text/plain",
			want:    fallback,
		},
		{
			name:    "invalid mime type no encoding",
			content: content,
			mime:    "text",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using require.True here because require.Same is not yet released.
			// We're comparing pointer values and the require.Equal will
			// dereference and compare the pointed to values rather than check
			// if the memory addresses are the same. Once require.Same is released
			// this test should convert to using that.
			require.True(t, tt.want == tt.content.Get(tt.mime))
		})
	}
}
