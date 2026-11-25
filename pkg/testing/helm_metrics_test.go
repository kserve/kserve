/*
Copyright 2026 The KServe Authors.

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

package testing

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControllerMetricsHelmRender(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not available")
	}

	root := ProjectRoot()
	const deployment = "kserve-controller-manager"
	for _, tc := range []struct {
		name string
		port string
		sets []string
	}{
		{name: "default port", port: "8080"},
		{name: "custom port", port: "18080", sets: []string{"kserve.controller.metricsBindPort=18080"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderChart(t, helm, root, "charts/kserve-resources", tc.sets...)
			require.Contains(t, managerArgs(t, manifest, "Deployment", deployment), "--metrics-addr=127.0.0.1:"+tc.port)

			proxyArgs := containerArgs(t, manifest, "Deployment", deployment, "kube-rbac-proxy")
			require.Contains(t, proxyArgs, "--secure-listen-address=0.0.0.0:8443")
			var upstreamArgs []string
			for _, arg := range proxyArgs {
				if strings.HasPrefix(arg, "--upstream=") {
					upstreamArgs = append(upstreamArgs, arg)
				}
			}
			require.Equal(t, []string{"--upstream=http://127.0.0.1:" + tc.port + "/"}, upstreamArgs)
		})
	}
}
