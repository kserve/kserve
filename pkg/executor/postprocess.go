package executor

import (
	"net/http"
	"net/url"
)

// postprocess
func (eh *executorHandler) postprocess(r *http.Request, b []byte) ([]byte, error) {

	target := &url.URL{
		Scheme: "http",
		Host:   eh.postprocessHost,
		Path:   r.URL.Path,
	}
	b, err := eh.callServer(target, b, r.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	return b, nil
}
