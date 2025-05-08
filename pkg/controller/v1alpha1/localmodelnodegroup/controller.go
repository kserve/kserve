/*
Copyright 2025 The KServe Authors.

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

// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups/finalizers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups,verbs=get;list;watch;update;patch

package localmodelnodegroup

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	finalizerName = "localmodelnodegroup.kserve.io/finalizer"
	// ServiceAccount is created during the installation of KServe
	serviceAccountName = "kserve-localmodelnode-agent"
	agentSuffix        = "-agent"

	// Component labels
	pvComponent        = "localmodelnode-agent-pv"
	pvcComponent       = "localmodelnode-agent-pvc"
	daemonsetComponent = "localmodelnode-agent"

	// Common label keys
	appNameLabel      = "app.kubernetes.io/name"
	appInstanceLabel  = "app.kubernetes.io/instance"
	appManagedByLabel = "app.kubernetes.io/managed-by"
	appComponentLabel = "app.kubernetes.io/component"

	// Label values
	managedByValue = "kserve-localmodelnodegroup"
)

type LocalModelNodeGroupReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

func NewLocalModelNodeGroupReconciler(client client.Client, clientset *kubernetes.Clientset, log logr.Logger, scheme *runtime.Scheme) *LocalModelNodeGroupReconciler {
	return &LocalModelNodeGroupReconciler{
		Client:    client,
		Clientset: clientset,
		Log:       log.WithName("LocalModelNodeGroupReconciler"),
		Scheme:    scheme,
	}
}

// createPV creates a PersistentVolume for the LocalModelNodeGroup
func (r *LocalModelNodeGroupReconciler) createPV(ctx context.Context, nodeGroup v1alpha1.LocalModelNodeGroup) (*corev1.PersistentVolume, error) {
	persistentVolumes := r.Clientset.CoreV1().PersistentVolumes()
	name := nodeGroup.Name + agentSuffix

	if pv, err := persistentVolumes.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			// Create the PersistentVolume if it doesn't exist
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
					Labels: map[string]string{
						appNameLabel:      name,
						appInstanceLabel:  nodeGroup.Name,
						appManagedByLabel: managedByValue,
						appComponentLabel: pvComponent,
					},
				},
				Spec: nodeGroup.Spec.PersistentVolumeSpec,
			}
			if err := controllerutil.SetControllerReference(&nodeGroup, pv, r.Scheme); err != nil {
				r.Log.Error(err, "Failed to set controller reference for PersistentVolume", "name", pv.Name)
				return nil, err
			}
			if _, err := persistentVolumes.Create(ctx, pv, metav1.CreateOptions{}); err != nil {
				r.Log.Error(err, "Failed to create PersistentVolume", "name", pv.Name)
				return nil, err
			}
			return pv, nil
		} else {
			// If the error is not "not found", return the error
			r.Log.Error(err, "Failed to get PersistentVolume", "name", name)
			return nil, err
		}
	} else {
		return pv, nil
	}
}

// createPVC creates a PersistentVolumeClaim for the LocalModelNodeGroup
func (r *LocalModelNodeGroupReconciler) createPVC(ctx context.Context, nodeGroup *v1alpha1.LocalModelNodeGroup, pvName string) (*corev1.PersistentVolumeClaim, error) {
	name := nodeGroup.Name + agentSuffix
	persistentVolumeClaims := r.Clientset.CoreV1().PersistentVolumeClaims(constants.KServeNamespace)

	// Check if the PersistentVolumeClaim already exists
	if pvc, err := persistentVolumeClaims.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			// Create the PersistentVolumeClaim if it doesn't exist
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: constants.KServeNamespace,
					Labels: map[string]string{
						appNameLabel:      name,
						appInstanceLabel:  nodeGroup.Name,
						appManagedByLabel: managedByValue,
						appComponentLabel: pvcComponent,
					},
				},
				Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
			}
			pvc.Spec.VolumeName = pvName
			if err := controllerutil.SetControllerReference(nodeGroup, pvc, r.Scheme); err != nil {
				r.Log.Error(err, "Failed to set controller reference for PersistentVolumeClaim", "name", pvc.Name, "namespace", pvc.Namespace)
				return nil, err
			}
			if _, err := persistentVolumeClaims.Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
				r.Log.Error(err, "Failed to create PersistentVolumeClaim", "name", pvc.Name, "namespace", pvc.Namespace)
				return nil, err
			}
			return pvc, nil
		} else {
			// If the error is not "not found", return the error
			r.Log.Error(err, "Failed to get PersistentVolumeClaim", "name", name, "namespace", constants.KServeNamespace)
			return nil, err
		}
	} else {
		return pvc, nil
	}
}

func createLocalModelAgentDaemonSet(nodeGroup v1alpha1.LocalModelNodeGroup, localModelConfig v1beta1.LocalModelConfig, pvcName string) *appsv1.DaemonSet {
	agentName := nodeGroup.Name + agentSuffix
	agentLabels := map[string]string{
		appNameLabel:      agentName,
		appInstanceLabel:  nodeGroup.Name,
		appManagedByLabel: managedByValue,
		appComponentLabel: daemonsetComponent,
	}
	agent := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: constants.KServeNamespace,
			Labels:    agentLabels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: agentLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: agentLabels,
					Annotations: map[string]string{
						"kubectl.kubernetes.io/default-container": "manager",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           localModelConfig.LocalModelAgentImage,
							ImagePullPolicy: corev1.PullPolicy(localModelConfig.LocalModelAgentImagePullPolicy),
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								AllowPrivilegeEscalation: ptr.To(false),
								RunAsNonRoot:             ptr.To(true),
								ReadOnlyRootFilesystem:   ptr.To(true),
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(localModelConfig.LocalModelAgentCpuRequest),
									corev1.ResourceMemory: resource.MustParse(localModelConfig.LocalModelAgentMemoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(localModelConfig.LocalModelAgentCpuLimit),
									corev1.ResourceMemory: resource.MustParse(localModelConfig.LocalModelAgentMemoryLimit),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "models",
									MountPath: "/mnt/models",
									ReadOnly:  false,
								},
							},
						},
					},
					// Daemonset should only run on nodes that match the PV node selector
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: nodeGroup.Spec.PersistentVolumeSpec.NodeAffinity.Required.NodeSelectorTerms,
							},
						},
					},
					ServiceAccountName: serviceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
					},
					TerminationGracePeriodSeconds: ptr.To(int64(10)),
					Volumes: []corev1.Volume{
						{
							Name: "models",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
	}
	return agent
}

// semanticEquals checks if the desired and existing DaemonSet are semantically equal
// It compares the Spec, Labels, and Annotations of the DaemonSet
func semanticEquals(desired, existing *appsv1.DaemonSet) bool {
	return equality.Semantic.DeepEqual(&desired.Spec, &existing.Spec) &&
		equality.Semantic.DeepEqual(&desired.Labels, &existing.Labels) &&
		equality.Semantic.DeepEqual(&desired.Annotations, &existing.Annotations)
}

// Reconcile reconciles a LocalModelNodeGroup object. It creates local model node agent per NodeGroup.
// Step 1 - Check if the CR is in the deletion process
// Step 2 - Get the LocalModelConfig from the ConfigMap
// Step 3 - Create the PersistentVolume && PersistentVolumeClaim for model download
// Step 4 - Reconcile the Agent DaemonSet
func (r *LocalModelNodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("Reconciling LocalModelNodeGroup", "name", req.Name)
	nodeGroup := &v1alpha1.LocalModelNodeGroup{}
	if err := r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
		r.Log.Error(err, "Unable to fetch LocalModelNodeGroup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Step 1 - Checks if the CR is in the deletion process
	if nodeGroup.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !utils.Includes(nodeGroup.GetFinalizers(), finalizerName) {
			patch := client.MergeFrom(nodeGroup.DeepCopy())
			nodeGroup.SetFinalizers(append(nodeGroup.GetFinalizers(), finalizerName))
			if err := r.Patch(ctx, nodeGroup, patch); err != nil {
				r.Log.Error(err, "Unable to patch LocalModelNodeGroup with finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted, so if it has our finalizer, then lets
		// remove it and update the object. This is equivalent to unregistering
		// our finalizer.
		if utils.Includes(nodeGroup.GetFinalizers(), finalizerName) {
			patch := client.MergeFrom(nodeGroup.DeepCopy())
			nodeGroup.SetFinalizers(utils.RemoveString(nodeGroup.GetFinalizers(), finalizerName))
			if err := r.Patch(ctx, nodeGroup, patch); err != nil {
				r.Log.Error(err, "Unable to patch LocalModelNodeGroup without finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Step 2 - Get the LocalModelConfig from the ConfigMap
	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, r.Clientset)
	if err != nil {
		r.Log.Error(err, "Unable to fetch ConfigMap", "name", constants.InferenceServiceConfigMapName, "namespace", constants.KServeNamespace)
		return ctrl.Result{}, err
	}
	localmodelConfig, err := v1beta1.NewLocalModelConfig(configMap)
	if err != nil {
		r.Log.Error(err, "Unable to create LocalModelConfig from ConfigMap")
		return ctrl.Result{}, err
	}

	// Step 3 - Create the PersistentVolume && PersistentVolumeClaim for model download
	pv, err := r.createPV(ctx, *nodeGroup)
	if err != nil {
		// Error is already logged in createPV
		return ctrl.Result{}, err
	}

	pvc, err := r.createPVC(ctx, nodeGroup, pv.Name)
	if err != nil {
		// Error is already logged in createPVC
		return ctrl.Result{}, err
	}

	// Step 4 - Reconcile the Agent DaemonSet
	existing := &appsv1.DaemonSet{}
	desired := createLocalModelAgentDaemonSet(*nodeGroup, *localmodelConfig, pvc.Name)
	if err := controllerutil.SetControllerReference(nodeGroup, desired, r.Scheme); err != nil {
		r.Log.Error(err, "Failed to set controller reference for DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.KServeNamespace, Name: desired.Name}, existing); err != nil {
		if errors.IsNotFound(err) {
			// Create the DaemonSet if it doesn't exist
			if err := r.Create(ctx, desired); err != nil {
				r.Log.Error(err, "Failed to create DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
				return ctrl.Result{}, err
			}
			r.Log.Info("Created DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
			return ctrl.Result{}, nil
		} else {
			// If the error is not "not found", return the error
			r.Log.Error(err, "Failed to get DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
			return ctrl.Result{}, err
		}
	} else {
		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion

		// Do a dry-run update to avoid diffs generated by default values introduced by defaulter webhook.
		// This will populate our local daemonset object with any default values
		// that are present on the remote version.
		if err := r.Update(ctx, desired, client.DryRunAll); err != nil {
			r.Log.Error(err, "Failed to perform dry-run update of daemonset", "daemonset", desired.Name, "namespace", desired.Namespace)
			return ctrl.Result{}, err
		}
		// Update the DaemonSet if it is not in the desired state
		if !semanticEquals(desired, existing) {
			if err := r.Update(ctx, desired); err != nil {
				r.Log.Error(err, "Failed to update DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
				return ctrl.Result{}, err
			}
			r.Log.Info("Updated DaemonSet", "name", desired.Name, "namespace", desired.Namespace)
		}
	}
	// TODO: Update LocalModelNodeGroup status
	return ctrl.Result{}, nil
}

func (r *LocalModelNodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LocalModelNodeGroup{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}
