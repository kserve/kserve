package inferencelogger

import (
	"net/url"
)

type InferenceLogRequestType string

const (
	InferenceRequest  InferenceLogRequestType = "Request"
	InferenceResponse InferenceLogRequestType = "Response"
)

type LogRequest struct {
	url         *url.URL
	b           *[]byte
	contentType string
	reqType     InferenceLogRequestType
	id          string
	sourceUri   *url.URL
}
