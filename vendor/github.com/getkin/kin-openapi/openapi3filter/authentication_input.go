package openapi3filter

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type AuthenticationInput struct {
	RequestValidationInput *RequestValidationInput
	SecuritySchemeName     string
	SecurityScheme         *openapi3.SecurityScheme
	Scopes                 []string
}

func (input *AuthenticationInput) NewError(err error) error {
	if err == nil {
		scopes := input.Scopes
		if len(scopes) == 0 {
			err = fmt.Errorf("Security requirement '%s' failed",
				input.SecuritySchemeName)
		} else {
			err = fmt.Errorf("Security requirement '%s' (scopes: '%s') failed",
				input.SecuritySchemeName,
				strings.Join(input.Scopes, "', '"))
		}
	}
	return &RequestError{
		Input:  input.RequestValidationInput,
		Reason: "Authorization failed",
		Err:    err,
	}
}
