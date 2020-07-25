/*
Copyright 2020 kubeflow.org.

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

package explainer

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/components"
)

var _ components.Component = &Explainer{}

// Explainer reconciles resources for this component.
type Explainer struct {
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (e *Explainer) Reconcile(isvc *v1beta1.InferenceService) error {
	return nil
}
