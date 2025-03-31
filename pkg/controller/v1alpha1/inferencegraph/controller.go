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
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create;get;update;patch,resourceNames=kserve-inferencegraph-auth-verifiers
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=create;get;update;patch;watch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes/status,verbs=get
package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	osv1 "github.com/openshift/api/route/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	v1beta1api "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

// InferenceGraphReconciler reconciles a InferenceGraph object
type InferenceGraphReconciler struct {
	client.Client
	ClientConfig *rest.Config
	Clientset    kubernetes.Interface
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
}

// InferenceGraphState describes the Readiness of the InferenceGraph
type InferenceGraphState string

const (
	InferenceGraphControllerName string              = "inferencegraph-controller"
	InferenceGraphNotReadyState  InferenceGraphState = "InferenceGraphNotReady"
	InferenceGraphReadyState     InferenceGraphState = "InferenceGraphReady"
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
			panic(fmt.Errorf("Unable to unmarshall agent json string due to %w ", err))
		}
	}

	// Ensure that we set proper values for CPU/Memory Limit/Request
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

	// Fetch the InferenceGraph instance
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
	configMap, err := r.Clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
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
	deployConfig, err := v1beta1api.NewDeployConfig(r.Clientset)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "fails to create DeployConfig")
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if graph.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer.
		if !utils.Includes(graph.ObjectMeta.Finalizers, constants.InferenceGraphFinalizerName) {
			graph.ObjectMeta.Finalizers = append(graph.ObjectMeta.Finalizers, constants.InferenceGraphFinalizerName)
			patchYaml := "metadata:\n  finalizers: [" + strings.Join(graph.ObjectMeta.Finalizers, ",") + "]"
			patchJson, _ := yaml.YAMLToJSON([]byte(patchYaml))
			if err = r.Patch(ctx, graph, client.RawPatch(types.MergePatchType, patchJson)); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if utils.Includes(graph.ObjectMeta.Finalizers, constants.InferenceGraphFinalizerName) {
			// our finalizer is present, so lets cleanup resources
			if err = r.onDeleteCleanup(ctx, graph); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			graph.ObjectMeta.Finalizers = utils.RemoveString(graph.ObjectMeta.Finalizers, constants.InferenceGraphFinalizerName)
			patchYaml := "metadata:\n  finalizers: [" + strings.Join(graph.ObjectMeta.Finalizers, ",") + "]"
			patchJson, _ := yaml.YAMLToJSON([]byte(patchYaml))
			if err = r.Patch(ctx, graph, client.RawPatch(types.MergePatchType, patchJson)); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	deploymentMode := isvcutils.GetDeploymentMode(graph.Status.DeploymentMode, graph.ObjectMeta.Annotations, deployConfig)
	r.Log.Info("Inference graph deployment ", "deployment mode ", deploymentMode)
	if deploymentMode == constants.RawDeployment {
		// If the inference graph has auth enabled, create the supporting resources
		err = handleInferenceGraphRawAuthResources(ctx, r.Clientset, r.Scheme, graph)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile resources for auth verification")
		}

		// Create inference graph resources such as deployment, service, hpa in raw deployment mode
		deployment, url, err := handleInferenceGraphRawDeployment(r.Client, r.Clientset, r.Scheme, graph, routerConfig)

		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile inference graph raw deployment")
		}

		r.Log.Info("Inference graph raw", "deployment conditions", deployment.Status.Conditions)
		igAvailable := false
		for _, con := range deployment.Status.Conditions {
			if con.Type == appsv1.DeploymentAvailable {
				igAvailable = true
				break
			}
		}
		if !igAvailable {
			// If Deployment resource not yet available, IG is not available as well. Reconcile again.
			return reconcile.Result{Requeue: true}, errors.Wrapf(err,
				"Failed to find inference graph deployment  %s", graph.Name)
		}

		routeReconciler := OpenShiftRouteReconciler{
			Scheme: r.Scheme,
			Client: r.Client,
		}
		hostname, err := routeReconciler.Reconcile(ctx, graph)
		url.Host = hostname
		url.Scheme = "https"
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "fails to reconcile Route for InferenceGraph")
		}

		logger.Info("Inference graph raw before propagate status")
		PropagateRawStatus(&graph.Status, deployment, url)
	} else {
		// Abort if Knative Services are not available
		ksvcAvailable, checkKsvcErr := utils.IsCrdAvailable(r.ClientConfig, knservingv1.SchemeGroupVersion.String(), constants.KnativeServiceKind)
		if checkKsvcErr != nil {
			return reconcile.Result{}, checkKsvcErr
		}

		if !ksvcAvailable {
			r.Recorder.Event(graph, v1.EventTypeWarning, "ServerlessModeRejected",
				"It is not possible to use Serverless deployment mode when Knative Services are not available")
			return reconcile.Result{Requeue: false}, reconcile.TerminalError(fmt.Errorf("the resolved deployment mode of InferenceGraph '%s' is Serverless, but Knative Serving is not available", graph.Name))
		}

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
		// @TODO Need to check the status of all the graph components, find the inference services from all the nodes and collect the status
		for _, con := range ksvcStatus.Status.Conditions {
			if con.Type == apis.ConditionReady {
				if con.Status == "True" {
					graph.Status.URL = ksvcStatus.URL
				} else {
					graph.Status.URL = nil
				}
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
}

func inferenceGraphReadiness(status v1alpha1api.InferenceGraphStatus) bool {
	return status.Conditions != nil &&
		status.GetCondition(apis.ConditionReady) != nil &&
		status.GetCondition(apis.ConditionReady).Status == v1.ConditionTrue
}

func (r *InferenceGraphReconciler) onDeleteCleanup(ctx context.Context, graph *v1alpha1api.InferenceGraph) error {
	if err := removeAuthPrivilegesFromGraphServiceAccount(ctx, r.Clientset, graph); err != nil {
		return err
	}
	if err := deleteGraphServiceAccount(ctx, r.Clientset, graph); err != nil {
		return err
	}
	return nil
}

func (r *InferenceGraphReconciler) SetupWithManager(mgr ctrl.Manager, deployConfig *v1beta1api.DeployConfig) error {
	r.ClientConfig = mgr.GetConfig()

	ksvcFound, err := utils.IsCrdAvailable(r.ClientConfig, knservingv1.SchemeGroupVersion.String(), constants.KnativeServiceKind)
	if err != nil {
		return err
	}

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1api.InferenceGraph{}).
		Owns(&appsv1.Deployment{}).
		Owns(&osv1.Route{})

	if ksvcFound {
		ctrlBuilder = ctrlBuilder.Owns(&knservingv1.Service{})
	} else {
		r.Log.Info("The InferenceGraph controller won't watch serving.knative.dev/v1/Service resources because the CRD is not available.")
	}

	return ctrlBuilder.Complete(r)
}
