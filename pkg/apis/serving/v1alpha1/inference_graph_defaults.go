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

package v1alpha1

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/constants"
	utils "github.com/kserve/kserve/pkg/utils"
)

var defaulterLogger = logf.Log.WithName("inferencegraph-v1alpha1-mutating-webhook")

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false
// InferenceGraphDefaulter sets default values on InferenceGraph resources at admission time.
//
// Only minReplicas is defaulted here: both Knative (utils.SetAutoScalingAnnotations) and
// Standard (hpa reconciler) resolve nil to 1, so writing 1 is behaviour-neutral across modes.
// maxReplicas, scaleMetric, and timeout are intentionally left unset because their effective
// defaults differ by deployment mode (see hpa_reconciler.go and utils.go).
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating
// DeepCopy methods, as it is used only for temporary operations and does not need to be deeply copied.
type InferenceGraphDefaulter struct{}

// +kubebuilder:webhook:path=/mutate-serving-kserve-io-v1alpha1-inferencegraph,mutating=true,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=inferencegraphs,verbs=create;update,versions=v1alpha1,name=inferencegraph.kserve-webhook-server.defaulter,admissionReviewVersions=v1beta1
var _ webhook.CustomDefaulter = &InferenceGraphDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (d *InferenceGraphDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	ig, err := utils.Convert[*InferenceGraph](obj)
	if err != nil {
		defaulterLogger.Error(err, "Unable to convert object to InferenceGraph")
		return err
	}
	defaulterLogger.Info("Defaulting InferenceGraph", "name", ig.Name, "namespace", ig.Namespace)

	// Only default minReplicas. Do not touch maxReplicas / scaleMetric / timeout:
	// those resolve differently in Knative vs Standard mode.
	if ig.Spec.MinReplicas == nil {
		ig.Spec.MinReplicas = ptr.To(constants.DefaultMinReplicas)
	}
	return nil
}

// SetupInferenceGraphWebhookWithManager registers the InferenceGraph mutating and validating webhooks.
// Shared by cmd/manager and envtest so admission-path tests exercise production wiring.
func SetupInferenceGraphWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&InferenceGraph{}).
		WithDefaulter(&InferenceGraphDefaulter{}).
		WithValidator(&InferenceGraphValidator{}).
		Complete()
}
