package executor

import (
	"bytes"
	"github.com/go-logr/logr"
	"io/ioutil"
	"net/http"
	"net/url"
)

type executorHandler struct {
	log             logr.Logger
	preprocessHost  string
	predictorHost   string
	postprocessHost string
	transport       http.RoundTripper
}

func New(log logr.Logger, preprocess, predictor, postprocess string) http.Handler {
	return &executorHandler{
		log:             log,
		preprocessHost:  preprocess,
		predictorHost:   predictor,
		postprocessHost: postprocess,
	}
}

func (eh *executorHandler) post(url *url.URL, body []byte, contentType string) ([]byte, error) {
	eh.log.Info("Calling server", "url", url.String(), "contentType", contentType)
	response, err := http.Post(url.String(), contentType, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if err = response.Body.Close(); err != nil {
		return nil, err
	}
	return b, nil
}

// call optional preprocess, predict and optional postprocess
func (eh *executorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Preprocess
	if eh.preprocessHost != "" {
		if err := eh.preprocess(r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Predict
	b, err := eh.predict(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Postprocess
	if eh.postprocessHost != "" {
		b, err = eh.postprocess(r, b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Write final response
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
