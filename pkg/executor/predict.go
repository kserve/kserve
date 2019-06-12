package executor

import (
	"io/ioutil"
	"net/http"
	"net/url"
)

func (eh *executorHandler) predict(r *http.Request) ([]byte, error) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	eh.log.Info("Predict will send ", "msg", string(b))
	target := &url.URL{
		Scheme: "http",
		Host:   eh.predictorHost,
		Path:   r.URL.Path,
	}

	b, err = eh.callServer(target, b, r.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	return b, nil
}
