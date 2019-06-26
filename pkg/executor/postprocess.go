package executor

import (
	"net/http"
	"net/url"
)

// postprocess
func (eh *executorHandler) postprocess(r *http.Request, b []byte) ([]byte, error) {
	b, err := eh.post(&url.URL{
		Scheme: "http",
		Host:   eh.postprocessHost,
		Path:   r.URL.Path,
	}, b, r.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	return b, nil
}
