package openapi3filter

import (
	"context"
)

var DefaultOptions = &Options{}

type Options struct {
	ExcludeRequestBody    bool
	ExcludeResponseBody   bool
	IncludeResponseStatus bool
	AuthenticationFunc    func(c context.Context, input *AuthenticationInput) error
}
