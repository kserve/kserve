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

package resources

import (
	"github.com/knative/pkg/kmeta"
	"github.com/knative/serving/pkg/apis/autoscaling"
	pav1alpha1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	sv1a1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa/resources/names"
	"github.com/knative/serving/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const kpaLabelKey = autoscaling.GroupName + "/kpa"

// MakeMetricsService constructs a service that can be scraped for metrics.
func MakeMetricsService(pa *pav1alpha1.PodAutoscaler, selector map[string]string) *corev1.Service {

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.MetricsServiceName(pa.Name),
			Namespace: pa.Namespace,
			Labels: resources.UnionMaps(pa.GetLabels(), map[string]string{
				kpaLabelKey:               pa.Name,
				networking.ServiceTypeKey: string(networking.ServiceTypeMetrics),
			}),
			Annotations:     resources.CopyMap(pa.GetAnnotations()),
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(pa)},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       v1alpha1.ServiceQueueMetricsPortName,
				Protocol:   corev1.ProtocolTCP,
				Port:       v1alpha1.RequestQueueMetricsPort,
				TargetPort: intstr.FromString(sv1a1.RequestQueueMetricsPortName),
			}},
			Selector: selector,
		},
	}
}
