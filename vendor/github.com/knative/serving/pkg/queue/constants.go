/*
Copyright 2018 The Knative Authors

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

package queue

const (
	// Name is the name of the component.
	Name = "queue"

	// RequestQueueHealthPath specifies the path for health checks for
	// queue-proxy.
	RequestQueueHealthPath = "/health"

	// RequestQueueDrainPath specifies the path to wait until the proxy
	// server is shut down. Any subsequent calls to this endpoint after
	// the server has finished shutting down it will return immediately.
	// Main usage is to delay the termination of user-container until all
	// accepted requests have been processed.
	RequestQueueDrainPath = "/wait-for-drain"
)
