/*
Copyright 2019 The Knative Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"net/http"

	"github.com/knative/serving/pkg/network"
)

// HealthHandler handles responding to kubelet probes with a provided health check.
type HealthHandler struct {
	HealthCheck func() error
	NextHandler http.Handler
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if network.IsKubeletProbe(r) {
		if err := h.HealthCheck(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		return
	}

	h.NextHandler.ServeHTTP(w, r)
}
