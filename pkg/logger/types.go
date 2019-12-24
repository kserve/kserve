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
	Url         *url.URL
	Bytes       *[]byte
	ContentType string
	ReqType     LogRequestType
	Id          string
	SourceUri   *url.URL
	ModelId     string
}
