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

package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

var log = logf.Log.WithName("GraphKsvcReconciler")

type GraphKnativeServiceReconciler struct {
	client  client.Client
	scheme  *runtime.Scheme
	Service *knservingv1.Service
}

func NewGraphKnativeServiceReconciler(client client.Client,
	scheme *runtime.Scheme,
	ksvc *knservingv1.Service) *GraphKnativeServiceReconciler {
	return &GraphKnativeServiceReconciler{
		client:  client,
		scheme:  scheme,
		Service: ksvc,
	}
}

func reconcileKsvc(desired *knservingv1.Service, existing *knservingv1.Service) error {
	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	// knative mutator defaults the enableServiceLinks to false which would generate a diff despite no changes on desired knative service
	// https://github.com/knative/serving/blob/main/pkg/apis/serving/v1/revision_defaults.go#L134
	if desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks == nil &&
		existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks != nil &&
		*existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks == false {
		desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks = proto.Bool(false)
	}
	diff, err := kmp.SafeDiff(desired.Spec.ConfigurationSpec, existing.Spec.ConfigurationSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to diff inference graph knative service configuration spec")
	}
	log.Info("inference graph knative service configuration diff (-desired, +observed):", "diff", diff)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.Spec.Traffic = desired.Spec.Traffic
	return nil
}

func (r *GraphKnativeServiceReconciler) Reconcile() (*knservingv1.ServiceStatus, error) {
	desired := r.Service
	existing := &knservingv1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Creating inference graph knative service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		log.Info("Updating inference graph knative service", "namespace", desired.Namespace, "name", desired.Name)
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}
		if err := reconcileKsvc(desired, existing); err != nil {
			return err
		}
		return r.client.Update(context.TODO(), existing)
	})
	if err != nil {
		return &existing.Status, errors.Wrapf(err, "fails to update inference graph knative service")
	}
	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.Spec.RouteSpec, service.Spec.RouteSpec)
}

func createKnativeService(componentMeta metav1.ObjectMeta, graph *v1alpha1api.InferenceGraph, config *RouterConfig) *knservingv1.Service {
	bytes, err := json.Marshal(graph.Spec)
	if err != nil {
		return nil
	}
	annotations := componentMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	labels := componentMeta.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	if _, ok := annotations[autoscaling.MinScaleAnnotationKey]; !ok {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
	}

	// ksvc metadata.annotations
	ksvcAnnotations := make(map[string]string)

	if value, ok := annotations[constants.KnativeOpenshiftEnablePassthroughKey]; ok {
		ksvcAnnotations[constants.KnativeOpenshiftEnablePassthroughKey] = value
		delete(annotations, constants.KnativeOpenshiftEnablePassthroughKey)
	}

	labels = utils.Filter(componentMeta.Labels, func(key string) bool {
		return !utils.Includes(constants.RevisionTemplateLabelDisallowedList, key)
	})
	labels[constants.InferenceGraphLabel] = componentMeta.Name
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: ksvcAnnotations,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: config.Image,
									Args: []string{
										"--graph-json",
										string(bytes),
									},
									Resources: constructResourceRequirements(*graph, *config),
								},
							},
							Affinity: graph.Spec.Affinity,
						},
					},
				},
			},
		},
	}

	// Only adding this env variable "PROPAGATE_HEADERS" if router's headers config has the key "propagate"
	value, exists := config.Headers["propagate"]
	if exists {
		service.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = []v1.EnvVar{
			{
				Name:  constants.RouterHeadersPropagateEnvVar,
				Value: strings.Join(value, ","),
			},
		}
	}

	//Call setDefaults on desired knative service here to avoid diffs generated because knative defaulter webhook is
	//called when creating or updating the knative service
	service.SetDefaults(context.TODO())
	return service
}

func constructResourceRequirements(graph v1alpha1api.InferenceGraph, config RouterConfig) v1.ResourceRequirements {
	var specResources v1.ResourceRequirements
	if !reflect.ValueOf(graph.Spec.Resources).IsZero() {
		log.Info("Ignoring defaults for ResourceRequirements as spec has resources mentioned", "specResources", graph.Spec.Resources)
		specResources = v1.ResourceRequirements{
			Limits:   graph.Spec.Resources.Limits,
			Requests: graph.Spec.Resources.Requests,
		}
	} else {
		specResources = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(config.MemoryLimit),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(config.MemoryRequest),
			},
		}
	}
	return specResources
}
