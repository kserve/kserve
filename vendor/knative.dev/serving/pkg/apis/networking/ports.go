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

package networking

// The ports we setup on our services.
const (
	// ServiceHTTPPort is the port that we setup our Serving and Activator K8s services for
	// HTTP/1 endpoints.
	ServiceHTTPPort = 80

	// ServiceHTTP2Port is the port that we setup our Serving and Activator K8s services for
	// HTTP/2 endpoints.
	ServiceHTTP2Port = 81

	// BackendHTTPPort is the backend, i.e. `targetPort` that we setup for HTTP services.
	BackendHTTPPort = 8012

	// BackendHTTP2Port is the backend, i.e. `targetPort` that we setup for HTTP services.
	BackendHTTP2Port = 8013

	// QueueAdminPort specifies the port number for
	// health check and lifecycle hooks for queue-proxy.
	QueueAdminPort = 8022

	// AutoscalingQueueMetricsPort specifies the port number for metrics emitted
	// by queue-proxy for autoscaler.
	AutoscalingQueueMetricsPort = 9090

	// UserQueueMetricsPort specifies the port number for metrics emitted
	// by queue-proxy for end user.
	UserQueueMetricsPort = 9091

	// ServicePortNameHTTP1 is the name of the external port of the service for HTTP/1.1
	ServicePortNameHTTP1 = "http"

	// ServicePortNameH2C is the name of the external port of the service for HTTP/2
	ServicePortNameH2C = "http2"
)

// ServicePortName returns the port for the app level protocol.
func ServicePortName(proto ProtocolType) string {
	if proto == ProtocolH2C {
		return ServicePortNameH2C
	}
	return ServicePortNameHTTP1
}

// ServicePort chooses the service (load balancer) port for the public service.
func ServicePort(proto ProtocolType) int {
	if proto == ProtocolH2C {
		return ServiceHTTP2Port
	}
	return ServiceHTTPPort
}
