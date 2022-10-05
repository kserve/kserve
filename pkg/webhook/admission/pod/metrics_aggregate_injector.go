/*
Copyright 2021 The KServe Authors.

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
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

const (
	defaultPrometheusPort          = "8080"
	PrometheusPortAnnotationKey    = "prometheus.io/port"
	PrometheusPathAnnotationKey    = "prometheus.io/path"
	PrometheusPathAnnotationValue  = "/metrics"
	QueueProxyAggregateMetricsPort = "9088"
)

// InjectMetricsAggregator looks for the annotations to enable aggregate kserve-container and queue-proxy metrics and
// if specified, sets port-related EnvVars in queue-proxy and the aggregate prometheus annotation.
func InjectMetricsAggregator(pod *v1.Pod) error {
	for i, container := range pod.Spec.Containers {
		if container.Name == "queue-proxy" {
			if enableMetricAgg, ok := pod.ObjectMeta.Annotations[constants.EnableMetricAggregation]; ok && enableMetricAgg == "true" {
				// The kserve-container prometheus port is inherited from the ClusterServingRuntime YAML.
				// If no port is defined (transformer using python SDK), use the default port for the kserve-container.
				kserveContainerPromPort := defaultPrometheusPort
				if port, ok := pod.ObjectMeta.Annotations[constants.KserveContainerPrometheusPort]; ok {
					kserveContainerPromPort = port
				}

				// The kserveContainerPort is set as an EnvVar in the queue-proxy container
				// so that it knows which port to scrape from the kserve-container.
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: constants.KServeContainerPrometheusPortEnvVarKey, Value: kserveContainerPromPort})
				// Set the port that queue-proxy will use to expose the aggregate metrics.
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: constants.QueueProxyAggregatePrometheusPortEnvVarKey, Value: QueueProxyAggregateMetricsPort})

				// If SetPrometheusAggregateAnnotation is true, the pod annotations for prometheus port and path will be set. The scrape annotation is not set,
				// that is left for the user to configure.
				if setPromAnnotation, ok := pod.ObjectMeta.Annotations[constants.SetPrometheusAggregateAnnotation]; ok && setPromAnnotation == "true" {
					pod.ObjectMeta.Annotations[PrometheusPortAnnotationKey] = QueueProxyAggregateMetricsPort
					pod.ObjectMeta.Annotations[PrometheusPathAnnotationKey] = PrometheusPathAnnotationValue
				}
			}
		}
	}
	return nil
}
