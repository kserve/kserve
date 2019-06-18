package executor

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Call preprocess
func (eh *executorHandler) preprocess(r *http.Request) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	b, err = eh.post(&url.URL{
		Scheme: "http",
		Host:   eh.preprocessHost,
		Path:   r.URL.Path,
	}, b, r.Header.Get("Content-Type"))
	if err != nil {
		return err
	} else {
		r.Body = ioutil.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	return nil
}
