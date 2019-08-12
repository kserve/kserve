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

package v1alpha1

// GetStatus returns Status of an ClusterIngress
func (ci *ClusterIngress) GetStatus() *IngressStatus {
	return &ci.Status
}

// GetSpec returns Spec of an ClusterIngress
func (ci *ClusterIngress) GetSpec() *IngressSpec {
	return &ci.Spec
}

// SetStatus assigns ingress status
func (ci *ClusterIngress) SetStatus(status IngressStatus) {
	ci.Status = status
}

// SetSpec assigns ingress spec
func (ci *ClusterIngress) SetSpec(spec IngressSpec) {
	ci.Spec = spec
}
