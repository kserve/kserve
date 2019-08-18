/*
Copyright 2019 kubeflow.org.

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

package v1alpha2

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter
func (kfsvc *KFService) Default() {
	logger.Info("Defaulting KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	kfsvc.Spec.Default.ApplyDefaults()
	if kfsvc.Spec.Canary != nil {
		kfsvc.Spec.Canary.ApplyDefaults()
	}
}
