/*
Copyright 2022 The KServe Authors.

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

package pod

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	defaultKserveContainerPrometheusPort = "8080"
	MetricsAggregatorConfigMapKeyName    = "metricsAggregator"
)

type MetricsAggregator struct {
	EnableMetricAggregation  string `json:"enableMetricAggregation"`
	EnablePrometheusScraping string `json:"enablePrometheusScraping"`
}

func newMetricsAggregator(configMap *corev1.ConfigMap) *MetricsAggregator {
	ma := &MetricsAggregator{}

	if maConfigVal, ok := configMap.Data[MetricsAggregatorConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(maConfigVal), &ma)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall %v json string due to %w ", MetricsAggregatorConfigMapKeyName, err))
		}
	}

	return ma
}

func hasQueueProxy(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == constants.QueueProxyContainerName {
			return true
		}
	}
	return false
}

func kserveContainerPrometheusPort(pod *corev1.Pod) string {
	if port, ok := pod.Annotations[constants.KserveContainerPrometheusPortKey]; ok && port != "" {
		return port
	}
	return defaultKserveContainerPrometheusPort
}

func kserveContainerPrometheusPath(pod *corev1.Pod) string {
	if path, ok := pod.Annotations[constants.KServeContainerPrometheusPathKey]; ok && path != "" {
		return path
	}
	return constants.DefaultPrometheusPath
}

func setMetricAggregationEnvVarsAndPorts(pod *corev1.Pod) error {
	for i, container := range pod.Spec.Containers {
		if container.Name == constants.QueueProxyContainerName {
			// The kserve-container prometheus port/path is inherited from the ClusterServingRuntime YAML.
			// If no port is defined (transformer using python SDK), use the default port/path for the kserve-container.
			kserveContainerPromPort := kserveContainerPrometheusPort(pod)
			kserveContainerPromPath := kserveContainerPrometheusPath(pod)

			// The kserve container port/path is set as an EnvVar in the queue-proxy container
			// so that it knows which port/path to scrape from the kserve-container.
			pod.Spec.Containers[i].Env = utils.MergeEnvs(pod.Spec.Containers[i].Env, []corev1.EnvVar{
				{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: kserveContainerPromPort},
				{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: kserveContainerPromPath},
			})

			// Set the port that queue-proxy will use to expose the aggregate metrics.
			pod.Spec.Containers[i].Env = utils.MergeEnvs(pod.Spec.Containers[i].Env, []corev1.EnvVar{
				{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort},
			})
			aggrPort, err := utils.StringToInt32(constants.QueueProxyAggregatePrometheusMetricsPort)
			if err != nil {
				return err
			}
			pod.Spec.Containers[i].Ports = utils.AppendPortIfNotExists(pod.Spec.Containers[i].Ports, corev1.ContainerPort{
				Name:          constants.AggregateMetricsPortName,
				ContainerPort: aggrPort,
				Protocol:      "TCP",
			})
		}
	}
	return nil
}

// InjectMetricsAggregator looks for the annotations to enable aggregate kserve-container and queue-proxy metrics and
// if specified, sets port-related EnvVars in queue-proxy and the aggregate prometheus annotation.
func (ma *MetricsAggregator) InjectMetricsAggregator(pod *corev1.Pod) error {
	// Only set metric configs if the required annotations are set
	enableMetricAggregation, ok := pod.Annotations[constants.EnableMetricAggregation]
	if !ok {
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[constants.EnableMetricAggregation] = ma.EnableMetricAggregation
		enableMetricAggregation = ma.EnableMetricAggregation
	}
	if enableMetricAggregation == "true" {
		err := setMetricAggregationEnvVarsAndPorts(pod)
		if err != nil {
			return err
		}
	}

	// Handle setting the pod prometheus annotations
	setPromAnnotation, ok := pod.Annotations[constants.SetPrometheusAnnotation]
	if !ok {
		pod.Annotations[constants.SetPrometheusAnnotation] = ma.EnablePrometheusScraping
		setPromAnnotation = ma.EnablePrometheusScraping
	}
	if setPromAnnotation == "true" {
		// Preserve an explicit prometheus.io scrape annotation from the InferenceService
		// or serving runtime. The webhook previously overwrote these unconditionally.
		if _, hasPort := pod.Annotations[constants.PrometheusPortAnnotationKey]; !hasPort {
			var podPromPort string
			if hasQueueProxy(pod) {
				// Knative: scrape queue-proxy (or the aggregate port when aggregation is on).
				podPromPort = constants.DefaultPodPrometheusPort
				if enableMetricAggregation == "true" {
					podPromPort = constants.QueueProxyAggregatePrometheusMetricsPort
				}
			} else {
				// Standard/RawDeployment has no queue-proxy; scrape the model container.
				podPromPort = kserveContainerPrometheusPort(pod)
			}
			pod.Annotations[constants.PrometheusPortAnnotationKey] = podPromPort
		}
		if _, hasPath := pod.Annotations[constants.PrometheusPathAnnotationKey]; !hasPath {
			if hasQueueProxy(pod) {
				pod.Annotations[constants.PrometheusPathAnnotationKey] = constants.DefaultPrometheusPath
			} else {
				pod.Annotations[constants.PrometheusPathAnnotationKey] = kserveContainerPrometheusPath(pod)
			}
		}
	}

	return nil
}
