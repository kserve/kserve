package inferencelogger

import (
	"net/url"
)

type LogRequest struct {
	url         *url.URL
	b           *[]byte
	contentType string
}
