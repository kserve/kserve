/*
Copyright 2021 kubeflow.org.

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

// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=inferencegraphs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kubeflow.org,resources=inferencegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/proto"
	v1alpha1api "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// InferenceGraphReconciler reconciles a InferenceGraph object
type InferenceGraphReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

type RouterConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

func getRouterConfigs(configMap *v1.ConfigMap) (*RouterConfig, error) {

	routerConfig := &RouterConfig{}
	if agentConfigValue, ok := configMap.Data["router"]; ok {
		err := json.Unmarshal([]byte(agentConfigValue), &routerConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall agent json string due to %v ", err))
		}
	}

	//Ensure that we set proper values for CPU/Memory Limit/Request
	resourceDefaults := []string{routerConfig.MemoryRequest,
		routerConfig.MemoryLimit,
		routerConfig.CpuRequest,
		routerConfig.CpuLimit}
	for _, key := range resourceDefaults {
		_, err := resource.ParseQuantity(key)
		if err != nil {
			return routerConfig, fmt.Errorf("Failed to parse resource configuration for router: %q",
				err.Error())
		}
	}

	return routerConfig, nil
}

func (r *InferenceGraphReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	// Fetch the InferenceService instance
	graph := &v1alpha1api.InferenceGraph{}
	if err := r.Get(ctx, req.NamespacedName, graph); err != nil {
		if apierr.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	r.Log.Info("Reconciling inference graph", "apiVersion", graph.APIVersion, "graph", graph.Name)
	configMap := &v1.ConfigMap{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		r.Log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return reconcile.Result{}, err
	}
	routerConfig, err := getRouterConfigs(configMap)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Create service if does not exist
	desired := createKnativeService(graph.ObjectMeta, graph, routerConfig)
	existing := &knservingv1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if apierr.IsNotFound(err) {
			r.Log.Info("Creating knative service", "namespace", desired.Namespace, "name", desired.Name)
			err = r.Client.Create(context.TODO(), desired)
			if err != nil {
				return reconcile.Result{}, err
			} else {
				return reconcile.Result{}, nil
			}
		} else {
			return reconcile.Result{}, err
		}
	}
	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return reconcile.Result{}, nil
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
		return reconcile.Result{}, err
	}
	r.Log.Info("knative service configuration diff (-desired, +observed):", "diff", diff)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.Spec.Traffic = desired.Spec.Traffic
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		r.Log.Info("Updating knative service", "namespace", desired.Namespace, "name", desired.Name)
		return r.Client.Update(context.TODO(), existing)
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.updateStatus(graph); err != nil {
		r.Recorder.Eventf(graph, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
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

	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	labels := utils.Filter(componentMeta.Labels, func(key string) bool {
		return !utils.Includes(constants.RevisionTemplateLabelDisallowedList, key)
	})
	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentMeta.Name,
			Namespace: componentMeta.Namespace,
			Labels:    componentMeta.Labels,
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
								},
							},
						},
					},
				},
			},
		},
	}
	//Call setDefaults on desired knative service here to avoid diffs generated because knative defaulter webhook is
	//called when creating or updating the knative service
	service.SetDefaults(context.TODO())
	return service
}

func (r *InferenceGraphReconciler) updateStatus(desiredService *v1alpha1api.InferenceGraph) error {
	existingService := &v1alpha1api.InferenceGraph{}
	namespacedName := types.NamespacedName{Name: desiredService.Name, Namespace: desiredService.Namespace}
	if err := r.Get(context.TODO(), namespacedName, existingService); err != nil {
		return err
	}
	wasReady := inferenceGraphReadiness(existingService.Status)
	if equality.Semantic.DeepEqual(existingService.Status, desiredService.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if err := r.Status().Update(context.TODO(), desiredService); err != nil {
		r.Log.Error(err, "Failed to update InferenceGraph status", "InferenceGraph", desiredService.Name)
		r.Recorder.Eventf(desiredService, v1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for InferenceGraph %q: %v", desiredService.Name, err)
		return errors.Wrapf(err, "fails to update InferenceGraph status")
	} else {
		// If there was a difference and there was no error.
		isReady := inferenceGraphReadiness(desiredService.Status)
		if wasReady && !isReady { // Moved to NotReady State
			r.Recorder.Eventf(desiredService, v1.EventTypeWarning, string(v1alpha2.InferenceServiceNotReadyState),
				fmt.Sprintf("InferenceGraph [%v] is no longer Ready", desiredService.GetName()))
		} else if !wasReady && isReady { // Moved to Ready State
			r.Recorder.Eventf(desiredService, v1.EventTypeNormal, string(v1alpha2.InferenceServiceReadyState),
				fmt.Sprintf("InferenceGraph [%v] is Ready", desiredService.GetName()))
		}
	}
	return nil
}

func inferenceGraphReadiness(status v1alpha1api.InferenceGraphStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == v1.ConditionTrue
}

func (r *InferenceGraphReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.InferenceGraph{}).
		Owns(&knservingv1.Service{}).
		Complete(r)
}
