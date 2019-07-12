package openapi3_test

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestServerParamNames(t *testing.T) {
	server := &openapi3.Server{
		URL: "http://{x}.{y}.example.com",
	}
	values, err := server.ParameterNames()
	require.NoError(t, err)
	require.Exactly(t, []string{"x", "y"}, values)
}

func TestServerParamValuesWithPath(t *testing.T) {
	server := &openapi3.Server{
		URL: "http://{arg0}.{arg1}.example.com/a/b",
	}
	for input, expected := range map[string]*serverMatch{
		"http://x.example.com/a/b":                 nil,
		"http://x.y.example.com/":                  nil,
		"http://x.y.example.com/a/":                nil,
		"http://x.y.example.com/a/b":               newServerMatch("/", "x", "y"),
		"http://x.y.example.com/a/b/":              newServerMatch("/", "x", "y"),
		"http://x.y.example.com/a/b/c":             newServerMatch("/c", "x", "y"),
		"http://domain0.domain1.example.com/a/b/c": newServerMatch("/c", "domain0", "domain1"),
	} {
		t.Run(input, testServerParamValues(t, server, input, expected))
	}
}

func TestServerParamValuesNoPath(t *testing.T) {
	server := &openapi3.Server{
		URL: "https://{arg0}.{arg1}.example.com/",
	}
	for input, expected := range map[string]*serverMatch{
		"https://domain0.domain1.example.com/": newServerMatch("/", "domain0", "domain1"),
	} {
		t.Run(input, testServerParamValues(t, server, input, expected))
	}
}

func testServerParamValues(t *testing.T, server *openapi3.Server, input string, expected *serverMatch) func(*testing.T) {
	return func(t *testing.T) {
		args, remaining, ok := server.MatchRawURL(input)
		if expected == nil {
			require.False(t, ok)
			return
		}
		require.True(t, ok)

		actual := &serverMatch{
			Remaining: remaining,
			Args:      args,
		}
		require.Equal(t, expected, actual)
	}
}

type serverMatch struct {
	Remaining string
	Args      []string
}

func newServerMatch(remaining string, args ...string) *serverMatch {
	return &serverMatch{
		Remaining: remaining,
		Args:      args,
	}
}
