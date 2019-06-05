package executor

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Call a preprocessor and change the incoming request with its response
func (eh *executorHandler) preprocess(r *http.Request) error {
	target := &url.URL{
		Scheme: "http",
		Host:   eh.preprocessHost,
	}
	eh.log.Info("Calling preprocessor", "url", target.String())
	respPost, err := http.Post(target.String(), r.Header.Get("Content-Type"), r.Body)

	b, err := ioutil.ReadAll(respPost.Body) //Read html
	if err != nil {
		return err
	}
	if err = respPost.Body.Close(); err != nil {
		return err
	}

	r.Body = ioutil.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))

	return nil
}
