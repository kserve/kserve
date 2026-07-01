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

package fixture

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceOption ObjectOption[*corev1.Service]

func Service(name string, opts ...ServiceOption) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func WithGatewayLabel(gatewayName string) ServiceOption {
	return func(svc *corev1.Service) {
		if svc.Labels == nil {
			svc.Labels = make(map[string]string)
		}
		svc.Labels["gateway.networking.k8s.io/gateway-name"] = gatewayName
	}
}

func WithPorts(ports ...corev1.ServicePort) ServiceOption {
	return func(svc *corev1.Service) {
		svc.Spec.Ports = append(svc.Spec.Ports, ports...)
	}
}
