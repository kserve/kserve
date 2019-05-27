package handler

import (
	"github.com/go-logr/logr"
	//"go.opencensus.io/plugin/ochttp"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type orchestratorHandler struct {
	log         logr.Logger
	serviceFQDN string
	transport   http.RoundTripper
}

func New(log logr.Logger, serviceFQDN string) http.Handler {
	return &orchestratorHandler{
		log:         log,
		serviceFQDN: serviceFQDN,
		//transport: AutoTransport,
	}
}

// At present we just reverse proxy to the model but in future we will
// make calls to other components as described by the Spec and combine
// the responses into a single response.
func (oh *orchestratorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := &url.URL{
		Scheme: "http",
		Host:   "istio-ingressgateway.istio-system.svc.cluster.local",
	}

	host := oh.serviceFQDN

	// Update the headers to allow for SSL redirection
	r.Header.Set("Host", host)
	r.URL.Host = target.Host
	r.URL.Scheme = target.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = host

	oh.log.Info("About to proxy request")

	proxy := httputil.NewSingleHostReverseProxy(target)
	/*
		proxy.Transport = &ochttp.Transport{
			Base: oh.transport,
		}
	*/
	//proxy.FlushInterval = -1

	proxy.ServeHTTP(w, r)
}
