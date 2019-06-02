package handler

import (
	"bytes"
	"github.com/go-logr/logr"
	"io/ioutil"
	"strconv"

	"net/http"
	"net/http/httputil"
	"net/url"
)

type orchestratorHandler struct {
	log         logr.Logger
	preprocess  string
	predictor   string
	postprocess string
	transport   http.RoundTripper
}

func New(log logr.Logger, preprocess, predictor, postprocess string) http.Handler {
	return &orchestratorHandler{
		log:         log,
		preprocess:  preprocess,
		predictor:   predictor,
		postprocess: postprocess,
	}
}

// Return a postProcessor function which can be called by the rever proxy to change the response.
func (oh *orchestratorHandler) createPostProcessor() func(response *http.Response) error {

	f := func(resp *http.Response) (err error) {

		oh.log.Info("Calling post-processor")

		b, err := ioutil.ReadAll(resp.Body) //Read html
		if err != nil {
			return err
		}
		err = resp.Body.Close()
		if err != nil {
			return err
		}

		target := &url.URL{
			Scheme: "http",
			Host:   oh.postprocess,
		}
		reader := bytes.NewReader(b)
		oh.log.Info("Calling postprocessor", "url", target.String())
		respPost, err := http.Post(target.String(), resp.Header.Get("Content-Type"), reader)

		b, err = ioutil.ReadAll(respPost.Body) //Read html
		if err != nil {
			return err
		}
		err = respPost.Body.Close()
		if err != nil {
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

// Call a preprocessor and change the incoming request with its response
func (oh *orchestratorHandler) preProcess(r *http.Request) error {
	target := &url.URL{
		Scheme: "http",
		Host:   oh.preprocess,
	}
	oh.log.Info("Calling preprocessor", "url", target.String())
	respPost, err := http.Post(target.String(), r.Header.Get("Content-Type"), r.Body)

	b, err := ioutil.ReadAll(respPost.Body) //Read html
	if err != nil {
		return err
	}
	oh.log.Info("Received preprocess response ", "value", string(b))
	err = respPost.Body.Close()
	if err != nil {
		return err
	}

	r.Body = ioutil.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))

	return nil
}

// Create a reverse proxy to call the predictor with optional pre and post processing
func (oh *orchestratorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if len(oh.preprocess) > 0 {
		err := oh.preProcess(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	target := &url.URL{
		Scheme: "http",
		//Host:   "istio-ingressgateway.istio-system.svc.cluster.local",
		Host: oh.predictor,
	}

	r.URL.Host = target.Host
	r.URL.Scheme = target.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = oh.predictor

	oh.log.Info("About to proxy request", "host", oh.predictor)

	proxy := httputil.NewSingleHostReverseProxy(target)
	if len(oh.postprocess) > 0 {
		proxy.ModifyResponse = oh.createPostProcessor()
	}

	proxy.ServeHTTP(w, r)
}
