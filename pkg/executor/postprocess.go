package executor

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

// Return a postProcessor function which can be called by the rever proxy to change the response.
func (eh *executorHandler) createPostProcessor() func(response *http.Response) error {

	f := func(resp *http.Response) (err error) {

		eh.log.Info("Calling post-processor")

		b, err := ioutil.ReadAll(resp.Body) //Read html
		if err != nil {
			return err
		}

		if err = resp.Body.Close(); err != nil {
			return err
		}

		target := &url.URL{
			Scheme: "http",
			Host:   eh.postprocessHost,
		}
		reader := bytes.NewReader(b)
		eh.log.Info("Calling postprocessor", "url", target.String())
		respPost, err := http.Post(target.String(), resp.Header.Get("Content-Type"), reader)

		b, err = ioutil.ReadAll(respPost.Body)
		if err != nil {
			return err
		}

		if err = respPost.Body.Close(); err != nil {
			return err
		}

		body := ioutil.NopCloser(bytes.NewReader(b))
		resp.Body = body
		resp.ContentLength = int64(len(b))
		resp.Header.Set("Content-Length", strconv.Itoa(len(b)))
		return nil
	}
	return f
}
