package executor

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Call a preprocessor
func (eh *executorHandler) preprocess(r *http.Request) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	eh.log.Info("Preprocess will send ", "msg", string(b))
	target := &url.URL{
		Scheme: "http",
		Host:   eh.preprocessHost,
		Path:   r.URL.Path,
	}
	b, err = eh.callServer(target, b, r.Header.Get("Content-Type"))
	if err != nil {
		return err
	} else {
		eh.log.Info("Preprocess result ", "msg", string(b))
		r.Body = ioutil.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	return nil
}
