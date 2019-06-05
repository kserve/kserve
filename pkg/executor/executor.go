package executor

import (
	"github.com/go-logr/logr"
	"net/http"
	"net/http/httputil"
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

// Create a reverse proxy to call the predictorHost with optional pre and post processing
func (eh *executorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if eh.preprocessHost != "" {
		err := eh.preprocess(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	target := &url.URL{
		Scheme: "http",
		Host:   eh.predictorHost,
	}

	r.URL.Host = target.Host
	r.URL.Scheme = target.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = eh.predictorHost

	eh.log.Info("About to proxy request", "host", eh.predictorHost)

	proxy := httputil.NewSingleHostReverseProxy(target)
	if eh.postprocessHost != "" {
		proxy.ModifyResponse = eh.createPostProcessor()
	}

	proxy.ServeHTTP(w, r)
}
