package handler

import (
	"github.com/go-logr/logr"
	"net/http"

	"github.com/knative/serving/pkg/network"
)

// HealthHandler handles responding to kubelet probes with a provided health check.
type HealthHandler struct {
	Log         logr.Logger
	NextHandler http.Handler
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	h.Log.Info("Headers", "values", r.Header)

	if network.IsKubeletProbe(r) {
		h.Log.Info("Health request")
		w.WriteHeader(http.StatusOK)
		return
	}

	h.NextHandler.ServeHTTP(w, r)
}
