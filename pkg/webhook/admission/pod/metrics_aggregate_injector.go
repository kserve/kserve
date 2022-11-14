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
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

const (
	defaultKserveContainerPrometheusPort = "8080"
	MetricsAggregatorConfigMapKeyName    = "metricsAggregator"
)

type MetricsAggregator struct {
	EnableMetricAggregation  string `json:"enableMetricAggregation"`
	EnablePrometheusScraping string `json:"enablePrometheusScraping"`
}

func newMetricsAggregator(configMap *v1.ConfigMap) (*MetricsAggregator, error) {
	ma := &MetricsAggregator{}

	if maConfigVal, ok := configMap.Data[MetricsAggregatorConfigMapKeyName]; ok {
		err := json.Unmarshal([]byte(maConfigVal), &ma)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall %v json string due to %v ", MetricsAggregatorConfigMapKeyName, err))
		}
	}

	return ma, nil
}

func setMetricAggregationEnvVars(pod *v1.Pod) {
	for i, container := range pod.Spec.Containers {
		if container.Name == "queue-proxy" {
			// The kserve-container prometheus port/path is inherited from the ClusterServingRuntime YAML.
			// If no port is defined (transformer using python SDK), use the default port/path for the kserve-container.
			kserveContainerPromPort := defaultKserveContainerPrometheusPort
			if port, ok := pod.ObjectMeta.Annotations[constants.KserveContainerPrometheusPortKey]; ok {
				kserveContainerPromPort = port
			}

			kserveContainerPromPath := constants.DefaultPrometheusPath
			if path, ok := pod.ObjectMeta.Annotations[constants.KServeContainerPrometheusPathKey]; ok {
				kserveContainerPromPath = path
			}

			// The kserve container port/path is set as an EnvVar in the queue-proxy container
			// so that it knows which port/path to scrape from the kserve-container.
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: constants.KServeContainerPrometheusMetricsPortEnvVarKey, Value: kserveContainerPromPort})
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: constants.KServeContainerPrometheusMetricsPathEnvVarKey, Value: kserveContainerPromPath})

			// Set the port that queue-proxy will use to expose the aggregate metrics.
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: constants.QueueProxyAggregatePrometheusMetricsPortEnvVarKey, Value: constants.QueueProxyAggregatePrometheusMetricsPort})

		}
	}
}

// InjectMetricsAggregator looks for the annotations to enable aggregate kserve-container and queue-proxy metrics and
// if specified, sets port-related EnvVars in queue-proxy and the aggregate prometheus annotation.
func (ma *MetricsAggregator) InjectMetricsAggregator(pod *v1.Pod) error {
	//Only set metric configs if the required annotations are set
	enableMetricAggregation, ok := pod.ObjectMeta.Annotations[constants.EnableMetricAggregation]
	if !ok {
		if pod.ObjectMeta.Annotations == nil {
			pod.ObjectMeta.Annotations = make(map[string]string)
		}
		pod.ObjectMeta.Annotations[constants.EnableMetricAggregation] = ma.EnableMetricAggregation
		enableMetricAggregation = ma.EnableMetricAggregation
	}
	if enableMetricAggregation == "true" {
		setMetricAggregationEnvVars(pod)
	}

	// Handle setting the pod prometheus annotations
	setPromAnnotation, ok := pod.ObjectMeta.Annotations[constants.SetPrometheusAnnotation]
	if !ok {
		pod.ObjectMeta.Annotations[constants.SetPrometheusAnnotation] = ma.EnablePrometheusScraping
		setPromAnnotation = ma.EnablePrometheusScraping
	}
	if setPromAnnotation == "true" {
		// Set prometheus port to default queue proxy prometheus metrics port.
		// If enableMetricAggregation is true, set it as the queue proxy metrics aggregation port.
		podPromPort := constants.DefaultPodPrometheusPort
		if enableMetricAggregation == "true" {
			podPromPort = constants.QueueProxyAggregatePrometheusMetricsPort
		}
		pod.ObjectMeta.Annotations[constants.PrometheusPortAnnotationKey] = podPromPort
		pod.ObjectMeta.Annotations[constants.PrometheusPathAnnotationKey] = constants.DefaultPrometheusPath
	}

	return nil
}
