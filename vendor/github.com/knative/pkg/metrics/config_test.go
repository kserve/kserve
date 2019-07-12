/*
Copyright 2018 The Knative Authors.
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
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	. "github.com/knative/pkg/logging/testing"
)

const (
	servingDomain = "knative.dev/serving"
	badDomain     = "test.domain"
	testComponent = "testComponent"
	testProj      = "test-project"
	anotherProj   = "another-project"
)

var (
	errorTests = []struct {
		name        string
		ops         ExporterOptions
		expectedErr string
	}{{
		name: "empty config",
		ops: ExporterOptions{
			Domain:    servingDomain,
			Component: testComponent,
		},
		expectedErr: "metrics config map cannot be empty",
	}, {
		name: "unsupportedBackend",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination":    "unsupported",
				"metrics.stackdriver-project-id": testProj,
			},
			Domain:    servingDomain,
			Component: testComponent,
		},
		expectedErr: "unsupported metrics backend value \"unsupported\"",
	}, {
		name: "emptyDomain",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination": "prometheus",
			},
			Domain:    "",
			Component: testComponent,
		},
		expectedErr: "metrics domain cannot be empty",
	}, {
		name: "invalidComponent",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination": "prometheus",
			},
			Domain:    servingDomain,
			Component: "",
		},
		expectedErr: "metrics component name cannot be empty",
	}, {
		name: "invalidReportingPeriod",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination":      "prometheus",
				"metrics.reporting-period-seconds": "test",
			},
			Domain:    servingDomain,
			Component: testComponent,
		},
		expectedErr: "invalid metrics.reporting-period-seconds value \"test\"",
	}, {
		name: "invalidAllowStackdriverCustomMetrics",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination":              "stackdriver",
				"metrics.allow-stackdriver-custom-metrics": "test",
			},
			Domain:    servingDomain,
			Component: testComponent,
		},
		expectedErr: "invalid metrics.allow-stackdriver-custom-metrics value \"test\"",
	}, {
		name: "tooSmallPrometheusPort",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination": "prometheus",
			},
			Domain:         servingDomain,
			Component:      testComponent,
			PrometheusPort: 1023,
		},
		expectedErr: "invalid port 1023, should between 1024 and 65535",
	}, {
		name: "tooBigPrometheusPort",
		ops: ExporterOptions{
			ConfigMap: map[string]string{
				"metrics.backend-destination": "prometheus",
			},
			Domain:         servingDomain,
			Component:      testComponent,
			PrometheusPort: 65536,
		},
		expectedErr: "invalid port 65536, should between 1024 and 65535",
	}}
	successTests = []struct {
		name                string
		ops                 ExporterOptions
		expectedConfig      metricsConfig
		expectedNewExporter bool // Whether the config requires a new exporter compared to previous test case
	}{
		// Note the first unit test is skipped in TestUpdateExporterFromConfigMap since
		// unit test does not have application default credentials.
		{
			name: "stackdriverProjectIDMissing",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination": "stackdriver",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
			expectedNewExporter: true,
		}, {
			name: "backendKeyMissing",
			ops: ExporterOptions{
				ConfigMap: map[string]string{},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    5 * time.Second,
				prometheusPort:     defaultPrometheusPort,
			},
			expectedNewExporter: true,
		}, {
			name: "validStackdriver",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":    "stackdriver",
					"metrics.stackdriver-project-id": anotherProj,
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              anotherProj,
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
			expectedNewExporter: true,
		}, {
			name: "validPrometheus",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination": "prometheus",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    5 * time.Second,
				prometheusPort:     defaultPrometheusPort,
			},
			expectedNewExporter: true,
		}, {
			name: "validCapitalStackdriver",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":    "Stackdriver",
					"metrics.stackdriver-project-id": testProj,
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              testProj,
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
			expectedNewExporter: true,
		}, {
			name: "overriddenReportingPeriodPrometheus",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":      "prometheus",
					"metrics.reporting-period-seconds": "12",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    12 * time.Second,
				prometheusPort:     defaultPrometheusPort,
			},
			expectedNewExporter: true,
		}, {
			name: "overriddenReportingPeriodStackdriver",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":      "stackdriver",
					"metrics.stackdriver-project-id":   "test2",
					"metrics.reporting-period-seconds": "7",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              "test2",
				reportingPeriod:                   7 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
			expectedNewExporter: true,
		}, {
			name: "overriddenReportingPeriodStackdriver2",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":      "stackdriver",
					"metrics.stackdriver-project-id":   "test2",
					"metrics.reporting-period-seconds": "3",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              "test2",
				reportingPeriod:                   3 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
		}, {
			name: "emptyReportingPeriodPrometheus",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":      "prometheus",
					"metrics.reporting-period-seconds": "",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    5 * time.Second,
				prometheusPort:     defaultPrometheusPort,
			},
			expectedNewExporter: true,
		}, {
			name: "emptyReportingPeriodStackdriver",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":      "stackdriver",
					"metrics.stackdriver-project-id":   "test2",
					"metrics.reporting-period-seconds": "",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              "test2",
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
			expectedNewExporter: true,
		}, {
			name: "allowStackdriverCustomMetric",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination":              "stackdriver",
					"metrics.stackdriver-project-id":           "test2",
					"metrics.reporting-period-seconds":         "",
					"metrics.allow-stackdriver-custom-metrics": "true",
				},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				stackdriverProjectID:              "test2",
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
				allowStackdriverCustomMetrics:     true,
			},
		}, {
			name: "overridePrometheusPort",
			ops: ExporterOptions{
				ConfigMap: map[string]string{
					"metrics.backend-destination": "prometheus",
				},
				Domain:         servingDomain,
				Component:      testComponent,
				PrometheusPort: 9091,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    5 * time.Second,
				prometheusPort:     9091,
			},
			expectedNewExporter: true,
		}}
	envTests = []struct {
		name           string
		ops            ExporterOptions
		expectedConfig metricsConfig
	}{
		{
			name: "stackdriverFromEnv",
			ops: ExporterOptions{
				ConfigMap: map[string]string{},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:                            servingDomain,
				component:                         testComponent,
				backendDestination:                Stackdriver,
				reportingPeriod:                   60 * time.Second,
				isStackdriverBackend:              true,
				stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
				stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
			},
		}, {
			name: "validPrometheus",
			ops: ExporterOptions{
				ConfigMap: map[string]string{"metrics.backend-destination": "prometheus"},
				Domain:    servingDomain,
				Component: testComponent,
			},
			expectedConfig: metricsConfig{
				domain:             servingDomain,
				component:          testComponent,
				backendDestination: Prometheus,
				reportingPeriod:    5 * time.Second,
				prometheusPort:     defaultPrometheusPort,
			},
		}}
)

func TestGetMetricsConfig(t *testing.T) {
	for _, test := range errorTests {
		t.Run(test.name, func(t *testing.T) {
			_, err := getMetricsConfig(test.ops, TestLogger(t))
			if err.Error() != test.expectedErr {
				t.Errorf("Wanted err: %v, got: %v", test.expectedErr, err)
			}
		})
	}

	for _, test := range successTests {
		t.Run(test.name, func(t *testing.T) {
			mc, err := getMetricsConfig(test.ops, TestLogger(t))
			if err != nil {
				t.Errorf("Wanted valid config %v, got error %v", test.expectedConfig, err)
			}
			if !reflect.DeepEqual(*mc, test.expectedConfig) {
				t.Errorf("Wanted config %v, got config %v", test.expectedConfig, *mc)
			}
		})
	}
}

func TestGetMetricsConfig_fromEnv(t *testing.T) {
	os.Setenv(defaultBackendEnvName, "stackdriver")
	for _, test := range envTests {
		t.Run(test.name, func(t *testing.T) {
			mc, err := getMetricsConfig(test.ops, TestLogger(t))
			if err != nil {
				t.Errorf("Wanted valid config %v, got error %v", test.expectedConfig, err)
			}
			if !reflect.DeepEqual(*mc, test.expectedConfig) {
				t.Errorf("Wanted config %v, got config %v", test.expectedConfig, *mc)
			}
		})
	}
	os.Unsetenv(defaultBackendEnvName)
}

func TestIsNewExporterRequired(t *testing.T) {
	setCurMetricsConfig(nil)
	for _, test := range successTests {
		t.Run(test.name, func(t *testing.T) {
			mc, err := getMetricsConfig(test.ops, TestLogger(t))
			if err != nil {
				t.Errorf("Wanted valid config %v, got error %v", test.expectedConfig, err)
			}
			changed := isNewExporterRequired(mc)
			if changed != test.expectedNewExporter {
				t.Errorf("isMetricsConfigChanged=%v wanted %v", changed, test.expectedNewExporter)
			}
			setCurMetricsConfig(mc)
		})
	}

	setCurMetricsConfig(&metricsConfig{
		domain:             servingDomain,
		component:          testComponent,
		backendDestination: Prometheus})
	newConfig := &metricsConfig{
		domain:               servingDomain,
		component:            testComponent,
		backendDestination:   Prometheus,
		stackdriverProjectID: testProj,
	}
	changed := isNewExporterRequired(newConfig)
	if changed {
		t.Error("isNewExporterRequired should be false if stackdriver project ID changes for prometheus backend")
	}
}

func TestUpdateExporter(t *testing.T) {
	setCurMetricsConfig(nil)
	oldConfig := getCurMetricsConfig()
	for _, test := range successTests[1:] {
		t.Run(test.name, func(t *testing.T) {
			UpdateExporter(test.ops, TestLogger(t))
			mConfig := getCurMetricsConfig()
			if mConfig == oldConfig {
				t.Error("Expected metrics config change")
			}
			if !reflect.DeepEqual(*mConfig, test.expectedConfig) {
				t.Errorf("Expected config: %v; got config %v", test.expectedConfig, mConfig)
			}
			oldConfig = mConfig
		})
	}

	for _, test := range errorTests {
		t.Run(test.name, func(t *testing.T) {
			UpdateExporter(test.ops, TestLogger(t))
			mConfig := getCurMetricsConfig()
			if mConfig != oldConfig {
				t.Error("mConfig should not change")
			}
		})
	}
}

func TestUpdateExporter_doesNotCreateExporter(t *testing.T) {
	setCurMetricsConfig(nil)
	for _, test := range errorTests {
		t.Run(test.name, func(t *testing.T) {
			UpdateExporter(test.ops, TestLogger(t))
			mConfig := getCurMetricsConfig()
			if mConfig != nil {
				t.Error("mConfig should not be created")
			}
		})
	}
}
