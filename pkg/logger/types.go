package logger

import (
	"net/url"
)

type LogRequestType string

const (
	InferenceRequest  LogRequestType = "Request"
	InferenceResponse LogRequestType = "Response"
)

type LogRequest struct {
	url         *url.URL
	b           *[]byte
	contentType string
	reqType     LogRequestType
	id          string
	sourceUri   *url.URL
	modelId     string
}
