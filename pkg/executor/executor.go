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

func (eh *executorHandler) callServer(target *url.URL, payload []byte, contentType string) ([]byte, error) {
	reader := bytes.NewReader(payload)
	eh.log.Info("Calling server", "url", target.String(), "contentType", contentType)
	respPost, err := http.Post(target.String(), contentType, reader)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(respPost.Body)
	if err != nil {
		return nil, err
	}
	if err = respPost.Body.Close(); err != nil {
		return nil, err
	}
	return b, nil
}

// call optional preprocess, predict and optional postprocess
func (eh *executorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Preprocess
	if eh.preprocessHost != "" {
		err := eh.preprocess(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			eh.log.Info("Process ok ")
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
