package openapi3filter

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	errRouteMissingSwagger          = errors.New("Route is missing OpenAPI specification")
	errRouteMissingOperation        = errors.New("Route is missing OpenAPI operation")
	ErrAuthenticationServiceMissing = errors.New("Request validator doesn't have an authentication service defined")
)

type RouteError struct {
	Route  Route
	Reason string
}

func (err *RouteError) Error() string {
	return err.Reason
}

type RequestError struct {
	Input       *RequestValidationInput
	Parameter   *openapi3.Parameter
	RequestBody *openapi3.RequestBody
	Status      int
	Reason      string
	Err         error
}

func (err *RequestError) HTTPStatus() int {
	status := err.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	return status
}

func (err *RequestError) Error() string {
	reason := err.Reason
	if e := err.Err; e != nil {
		if len(reason) == 0 {
			reason = e.Error()
		} else {
			reason += ": " + e.Error()
		}
	}
	if v := err.Parameter; v != nil {
		return fmt.Sprintf("Parameter '%s' in %s has an error: %s", v.Name, v.In, reason)
	} else if v := err.RequestBody; v != nil {
		return fmt.Sprintf("Request body has an error: %s", reason)
	} else {
		return reason
	}
}

type ResponseError struct {
	Input  *ResponseValidationInput
	Reason string
	Err    error
}

func (err *ResponseError) Error() string {
	reason := err.Reason
	if e := err.Err; e != nil {
		if len(reason) == 0 {
			reason = e.Error()
		} else {
			reason += ": " + e.Error()
		}
	}
	return reason
}

type SecurityRequirementsError struct {
	SecurityRequirements openapi3.SecurityRequirements
	Errors               []error
}

func (err *SecurityRequirementsError) Error() string {
	return "Security requirements failed"
}
