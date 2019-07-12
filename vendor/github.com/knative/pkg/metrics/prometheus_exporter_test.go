/*
Copyright 2019 The Knative Authors.
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
package metrics

import (
	"testing"
	"time"

	. "github.com/knative/pkg/logging/testing"
)

func TestNewPrometheusExporter(t *testing.T) {
	testCases := []struct {
		name         string
		config       metricsConfig
		expectedAddr string
	}{{
		name: "port 9090",
		config: metricsConfig{
			domain:             servingDomain,
			component:          testComponent,
			backendDestination: Prometheus,
			prometheusPort:     9090,
		},
		expectedAddr: ":9090",
	}, {
		name: "port 9091",
		config: metricsConfig{
			domain:             servingDomain,
			component:          testComponent,
			backendDestination: Prometheus,
			prometheusPort:     9091,
		},
		expectedAddr: ":9091",
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := newPrometheusExporter(&tc.config, TestLogger(t))
			if err != nil {
				t.Error(err)
			}
			if e == nil {
				t.Fatal("expected a non-nil metrics exporter")
			}
			expectPromSrv(t, tc.expectedAddr)
		})
	}
}

func expectPromSrv(t *testing.T, expectedAddr string) {
	time.Sleep(200 * time.Millisecond)
	srv := getCurPromSrv()
	if srv == nil {
		t.Fatal("expected a server for prometheus exporter")
	}
	if got, want := srv.Addr, expectedAddr; got != want {
		t.Errorf("metrics port addresses diff, got=%v, want=%v", got, want)
	}
}

func expectNoPromSrv(t *testing.T) {
	time.Sleep(200 * time.Millisecond)
	srv := getCurPromSrv()
	if srv != nil {
		t.Error("expected no server for stackdriver exporter")
	}
}
