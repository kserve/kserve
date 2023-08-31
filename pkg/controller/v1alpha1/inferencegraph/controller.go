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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferencegraphs;inferencegraphs/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferencegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/util/retry"

	"github.com/go-logr/logr"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// InferenceGraphReconciler reconciles a InferenceGraph object
type InferenceGraphReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// InferenceGraphState describes the Readiness of the InferenceGraph
type InferenceGraphState string

const (
	InferenceGraphNotReadyState InferenceGraphState = "InferenceGraphNotReady"
	InferenceGraphReadyState    InferenceGraphState = "InferenceGraphReady"
)

type RouterConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	/*
		Example of how to add headers in router config:
		headers: {
		 "propagate": [
			"Custom-Header1",
			"Custom-Header2"
		  ]
		}
		Note: Making Headers, a map of strings, gives the flexibility to extend it in the future to support adding more
		operations on headers. For example: Similar to "propagate" operation, one can add "transform" operation if they
		want to transform headers keys or values before passing down to nodes.
	*/
	Headers map[string][]string `json:"headers"`
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
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
	if err != nil {
		r.Log.Error(err, "Failed to find config map", "name", constants.InferenceServiceConfigMapName)
		return reconcile.Result{}, err
	}
	routerConfig, err := getRouterConfigs(configMap)
	if err != nil {
		return reconcile.Result{}, err
	}
	// resolve service urls
	for node, router := range graph.Spec.Nodes {
		for i, route := range router.Steps {
			isvc := v1beta1.InferenceService{}
			if route.ServiceName != "" {
				err := r.Client.Get(ctx, types.NamespacedName{Namespace: graph.Namespace, Name: route.ServiceName}, &isvc)
				if err == nil {
					if graph.Spec.Nodes[node].Steps[i].ServiceURL == "" {
						serviceUrl, err := isvcutils.GetPredictorEndpoint(&isvc)
						if err == nil {
							graph.Spec.Nodes[node].Steps[i].ServiceURL = serviceUrl
						} else {
							r.Log.Info("inference service is not ready", "name", route.ServiceName)
							return reconcile.Result{Requeue: true}, errors.Wrapf(err, "service %s is not ready", route.ServiceName)
						}
					}

				} else {
					r.Log.Info("inference service is not found", "name", route.ServiceName)
					return reconcile.Result{Requeue: true}, errors.Wrapf(err, "Failed to find graph service %s", route.ServiceName)
				}
			}
		}
	}
	deployConfig, err := v1beta1api.NewDeployConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create DeployConfig")
	}

	deploymentMode := isvcutils.GetDeploymentMode(graph.ObjectMeta.Annotations, deployConfig)
	r.Log.Info("Inference service deployment mode ", "deployment mode ", deploymentMode)
	if deploymentMode == constants.RawDeployment {
		err := fmt.Errorf("RawDeployment mode is not supported for InferenceGraph")
		r.Log.Error(err, "name", graph.GetName())
		return reconcile.Result{}, err
	}
	//@TODO check raw deployment mode
	desired := createKnativeService(graph.ObjectMeta, graph, routerConfig)
	err = controllerutil.SetControllerReference(graph, desired, r.Scheme)
	if err != nil {
		return reconcile.Result{}, err
	}
	knativeReconciler := NewGraphKnativeServiceReconciler(r.Client, r.Scheme, desired)
	ksvcStatus, err := knativeReconciler.Reconcile()
	if err != nil {
		r.Log.Error(err, "failed to reconcile inference graph ksvc", "name", graph.GetName())
		return reconcile.Result{}, errors.Wrapf(err, "fails to reconcile inference graph ksvc")
	}

	r.Log.Info("updating inference graph status", "status", ksvcStatus)
	graph.Status.Conditions = ksvcStatus.Status.Conditions
	//@TODO Need to check the status of all the graph components, find the inference services from all the nodes and collect the status
	for _, con := range ksvcStatus.Status.Conditions {
		if con.Type == apis.ConditionReady {
			if con.Status == "True" {
				graph.Status.URL = ksvcStatus.URL
			} else {
				graph.Status.URL = nil
			}
		}
	}
	if err := r.updateStatus(graph); err != nil {
		r.Recorder.Eventf(graph, v1.EventTypeWarning, "InternalError", err.Error())
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InferenceGraphReconciler) updateStatus(desiredGraph *v1alpha1api.InferenceGraph) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		graph := &v1alpha1api.InferenceGraph{}
		namespacedName := types.NamespacedName{Name: desiredGraph.Name, Namespace: desiredGraph.Namespace}
		if err := r.Get(context.TODO(), namespacedName, graph); err != nil {
			return err
		}

		wasReady := inferenceGraphReadiness(graph.Status)
		if equality.Semantic.DeepEqual(graph.Status, desiredGraph.Status) {
			// If we didn't change anything then don't call updateStatus.
			// This is important because the copy we loaded from the informer's
			// cache may be stale and we don't want to overwrite a prior update
			// to status with this stale state.
		} else if err := r.Status().Update(context.TODO(), desiredGraph); err != nil {
			if apierr.IsConflict(err) {
				return err
			}
			r.Log.Error(err, "Failed to update InferenceGraph status", "InferenceGraph", desiredGraph.Name)
			r.Recorder.Eventf(desiredGraph, v1.EventTypeWarning, "UpdateFailed",
				"Failed to update status for InferenceGraph %q: %v", desiredGraph.Name, err)
			return errors.Wrapf(err, "fails to update InferenceGraph status")
		} else {
			r.Log.Info("updated InferenceGraph status", "InferenceGraph", desiredGraph.Name)
			// If there was a difference and there was no error.
			isReady := inferenceGraphReadiness(desiredGraph.Status)
			if wasReady && !isReady { // Moved to NotReady State
				r.Recorder.Eventf(desiredGraph, v1.EventTypeWarning, string(InferenceGraphNotReadyState),
					fmt.Sprintf("InferenceGraph [%v] is no longer Ready", desiredGraph.GetName()))
			} else if !wasReady && isReady { // Moved to Ready State
				r.Recorder.Eventf(desiredGraph, v1.EventTypeNormal, string(InferenceGraphReadyState),
					fmt.Sprintf("InferenceGraph [%v] is Ready", desiredGraph.GetName()))
			}
		}
		return nil
	})
	return err
}

func inferenceGraphReadiness(status v1alpha1api.InferenceGraphStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == v1.ConditionTrue
}

func (r *InferenceGraphReconciler) SetupWithManager(mgr ctrl.Manager, deployConfig *v1beta1api.DeployConfig) error {
	if deployConfig.DefaultDeploymentMode == string(constants.RawDeployment) {
		return ctrl.NewControllerManagedBy(mgr).
			For(&v1alpha1api.InferenceGraph{}).
			Owns(&appsv1.Deployment{}).
			Complete(r)
	} else {
		return ctrl.NewControllerManagedBy(mgr).
			For(&v1alpha1api.InferenceGraph{}).
			Owns(&knservingv1.Service{}).
			Complete(r)
	}
}
