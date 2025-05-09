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

package localmodelnodegroup

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestCreatePV(t *testing.T) {
	// Setup common test resources
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	// Create a test LocalModelNodeGroup
	nodeGroup := v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-group",
		},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/mnt/data",
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/hostname",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Define the PV name that we expect to be created
	expectedPVName := "test-node-group-agent"

	t.Run("successfully create PV when it doesn't exist", func(t *testing.T) {
		// Setup fake clientset
		clientset := fake.NewSimpleClientset()

		// Create the reconciler with NewLocalModelNodeGroupReconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pv, err := reconciler.createPV(t.Context(), nodeGroup)

		// Check results
		require.NoError(t, err, "Expected no error when creating PV")
		require.NotNil(t, pv, "Expected PV to be returned")
		require.Equal(t, expectedPVName, pv.Name, "Expected PV name to match")

		// Verify the PV was created with the correct values
		createdPV, err := clientset.CoreV1().PersistentVolumes().Get(t.Context(), expectedPVName, metav1.GetOptions{})
		require.NoError(t, err, "Expected to find the created PV")
		require.Equal(t, expectedPVName, createdPV.Name, "Expected created PV name to match")
		require.Equal(t, managedByValue, createdPV.Labels[appManagedByLabel], "Expected managed-by label to be set correctly")
		require.Equal(t, nodeGroup.Spec.PersistentVolumeSpec.Capacity, createdPV.Spec.Capacity, "Expected capacity to match")
	})

	t.Run("return existing PV when it already exists", func(t *testing.T) {
		// Create a pre-existing PV
		existingPV := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedPVName,
				Labels: map[string]string{
					appNameLabel:      expectedPVName,
					appInstanceLabel:  nodeGroup.Name,
					appManagedByLabel: managedByValue,
					appComponentLabel: pvComponent,
				},
			},
			Spec: nodeGroup.Spec.PersistentVolumeSpec,
		}
		clientset := fake.NewSimpleClientset(existingPV)

		// Create the reconciler with NewLocalModelNodeGroupReconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pv, err := reconciler.createPV(t.Context(), nodeGroup)

		// Check results
		require.NoError(t, err, "Expected no error when getting existing PV")
		require.NotNil(t, pv, "Expected PV to be returned")
		require.Equal(t, expectedPVName, pv.Name, "Expected PV name to match")
	})

	t.Run("handle error when setting controller reference fails", func(t *testing.T) {
		// Setup fake clientset with a reactor that will cause SetControllerReference to fail
		clientset := fake.NewSimpleClientset()

		// Create a mock scheme that will cause SetControllerReference to fail
		badScheme := runtime.NewScheme() // Intentionally don't register types

		// Create the reconciler with NewLocalModelNodeGroupReconciler and bad scheme
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), badScheme)

		// Call the function
		pv, err := reconciler.createPV(t.Context(), nodeGroup)

		// Check results
		require.Error(t, err, "Expected error when setting controller reference fails")
		require.Nil(t, pv, "Expected no PV to be returned")
	})

	t.Run("handle error when creating PV fails", func(t *testing.T) {
		// Setup fake clientset with a reactor that will return an error when creating a PV
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", "persistentvolumes", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("failed to create PV")
		})

		// Create the reconciler with NewLocalModelNodeGroupReconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pv, err := reconciler.createPV(t.Context(), nodeGroup)

		// Check results
		require.Error(t, err, "Expected error when creating PV fails")
		require.Nil(t, pv, "Expected no PV to be returned")
		require.Contains(t, err.Error(), "failed to create PV", "Expected error message to indicate creation failure")
	})

	t.Run("handle error when getting PV fails", func(t *testing.T) {
		// Setup fake clientset with a reactor that will return an error when getting a PV
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("get", "persistentvolumes", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, &k8serrors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Reason:  metav1.StatusReasonInternalError,
					Message: "internal server error",
				},
			}
		})

		// Create the reconciler with NewLocalModelNodeGroupReconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pv, err := reconciler.createPV(t.Context(), nodeGroup)

		// Check results
		require.Error(t, err, "Expected error when getting PV fails")
		require.Nil(t, pv, "Expected no PV to be returned")
	})
}

func TestCreatePVC(t *testing.T) {
	// Setup common test resources
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	if err != nil {
		t.Fatal(err)
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test LocalModelNodeGroup
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-group",
		},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
	}

	pvName := "test-pv-name"
	expectedPVCName := "test-node-group-agent"

	t.Run("successfully create PVC when it doesn't exist", func(t *testing.T) {
		// Setup fake clientset
		clientset := fake.NewSimpleClientset()

		// Create the reconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pvc, err := reconciler.createPVC(t.Context(), nodeGroup, pvName)

		// Check results
		require.NoError(t, err, "Expected no error when creating PVC")
		require.NotNil(t, pvc, "Expected PVC to be returned")
		require.Equal(t, expectedPVCName, pvc.Name, "Expected PVC name to match")

		// Verify the PVC was created with the correct values
		createdPVC, err := clientset.CoreV1().PersistentVolumeClaims(constants.KServeNamespace).Get(t.Context(), expectedPVCName, metav1.GetOptions{})
		require.NoError(t, err, "Expected to find the created PVC")
		require.Equal(t, expectedPVCName, createdPVC.Name, "Expected created PVC name to match")
		require.Equal(t, pvName, createdPVC.Spec.VolumeName, "Expected PVC to reference correct PV")
		require.Equal(t, managedByValue, createdPVC.Labels[appManagedByLabel], "Expected managed-by label to be set correctly")
	})

	t.Run("return existing PVC when it already exists", func(t *testing.T) {
		// Create a pre-existing PVC
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedPVCName,
				Namespace: constants.KServeNamespace,
				Labels: map[string]string{
					appNameLabel:      expectedPVCName,
					appInstanceLabel:  nodeGroup.Name,
					appManagedByLabel: managedByValue,
					appComponentLabel: pvcComponent,
				},
			},
			Spec: nodeGroup.Spec.PersistentVolumeClaimSpec,
		}
		clientset := fake.NewSimpleClientset(existingPVC)

		// Create the reconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pvc, err := reconciler.createPVC(t.Context(), nodeGroup, pvName)

		// Check results
		require.NoError(t, err, "Expected no error when getting existing PVC")
		require.NotNil(t, pvc, "Expected PVC to be returned")
		require.Equal(t, expectedPVCName, pvc.Name, "Expected PVC name to match")
	})

	t.Run("handle error when setting controller reference fails", func(t *testing.T) {
		// Setup fake clientset
		clientset := fake.NewSimpleClientset()

		// Create a mock scheme that will cause SetControllerReference to fail
		badScheme := runtime.NewScheme() // Intentionally don't register types

		// Create the reconciler with bad scheme
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), badScheme)

		// Call the function
		pvc, err := reconciler.createPVC(t.Context(), nodeGroup, pvName)

		// Check results
		require.Error(t, err, "Expected error when setting controller reference fails")
		require.Nil(t, pvc, "Expected no PVC to be returned")
	})

	t.Run("handle error when creating PVC fails", func(t *testing.T) {
		// Setup fake clientset with a reactor that will return an error when creating a PVC
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", "persistentvolumeclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("failed to create PVC")
		})

		// Create the reconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pvc, err := reconciler.createPVC(t.Context(), nodeGroup, pvName)

		// Check results
		require.Error(t, err, "Expected error when creating PVC fails")
		require.Nil(t, pvc, "Expected no PVC to be returned")
		require.Contains(t, err.Error(), "failed to create PVC", "Expected error message to indicate creation failure")
	})

	t.Run("handle error when getting PVC fails", func(t *testing.T) {
		// Setup fake clientset with a reactor that will return an error when getting a PVC
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("get", "persistentvolumeclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, &k8serrors.StatusError{
				ErrStatus: metav1.Status{
					Status:  metav1.StatusFailure,
					Reason:  metav1.StatusReasonInternalError,
					Message: "internal server error",
				},
			}
		})

		// Create the reconciler
		reconciler := NewLocalModelNodeGroupReconciler(nil, clientset, logr.Discard(), scheme)

		// Call the function
		pvc, err := reconciler.createPVC(t.Context(), nodeGroup, pvName)

		// Check results
		require.Error(t, err, "Expected error when getting PVC fails")
		require.Nil(t, pvc, "Expected no PVC to be returned")
	})
}

func TestCreateLocalModelAgentDaemonSet(t *testing.T) {
	// Setup test data
	nodeGroup := v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-group",
		},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/hostname",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	localModelConfig := v1beta1.LocalModelConfig{
		LocalModelAgentImage:           "kserve/agent:latest",
		LocalModelAgentImagePullPolicy: "IfNotPresent",
		LocalModelAgentCpuRequest:      "100m",
		LocalModelAgentMemoryRequest:   "200Mi",
		LocalModelAgentCpuLimit:        "500m",
		LocalModelAgentMemoryLimit:     "500Mi",
	}

	pvcName := "test-pvc"
	expectedDaemonSetName := "test-node-group-agent"

	t.Run("validate daemonset creation with correct configuration", func(t *testing.T) {
		// Create the daemonset
		ds := createLocalModelAgentDaemonSet(nodeGroup, localModelConfig, pvcName)

		// Check basic metadata
		require.Equal(t, expectedDaemonSetName, ds.Name, "DaemonSet name should match")
		require.Equal(t, constants.KServeNamespace, ds.Namespace, "DaemonSet namespace should be KServe namespace")

		// Check labels
		require.Equal(t, expectedDaemonSetName, ds.Labels[appNameLabel], "DaemonSet should have correct name label")
		require.Equal(t, nodeGroup.Name, ds.Labels[appInstanceLabel], "DaemonSet should have correct instance label")
		require.Equal(t, managedByValue, ds.Labels[appManagedByLabel], "DaemonSet should have correct managed-by label")
		require.Equal(t, daemonsetComponent, ds.Labels[appComponentLabel], "DaemonSet should have correct component label")

		// Check selector
		require.Equal(t, ds.Labels, ds.Spec.Selector.MatchLabels, "Selector should match labels")

		// Check pod spec
		podSpec := ds.Spec.Template.Spec

		// Check node affinity
		require.NotNil(t, podSpec.Affinity, "Pod affinity should be set")
		require.NotNil(t, podSpec.Affinity.NodeAffinity, "Node affinity should be set")
		require.NotNil(t, podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution, "Required node selector should be set")

		// Check that node affinity matches the PV node affinity
		expectedTerms := nodeGroup.Spec.PersistentVolumeSpec.NodeAffinity.Required.NodeSelectorTerms
		actualTerms := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		require.Equal(t, expectedTerms, actualTerms, "Node selector terms should match PV node affinity")

		// Check service account
		require.Equal(t, serviceAccountName, podSpec.ServiceAccountName, "Service account name should be correct")

		// Check security context
		require.NotNil(t, podSpec.SecurityContext, "Pod security context should be set")
		require.True(t, *podSpec.SecurityContext.RunAsNonRoot, "Pod should run as non-root")

		// Check volumes
		require.Len(t, podSpec.Volumes, 1, "Should have one volume")
		require.Equal(t, "models", podSpec.Volumes[0].Name, "Volume name should be 'models'")
		require.NotNil(t, podSpec.Volumes[0].VolumeSource.PersistentVolumeClaim, "Volume source should be PVC")
		require.Equal(t, pvcName, podSpec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName, "PVC name should match")

		// Check container
		require.Len(t, podSpec.Containers, 1, "Should have one container")
		container := podSpec.Containers[0]

		// Check container name
		require.Equal(t, "manager", container.Name, "Container name should be 'manager'")

		// Check image settings
		require.Equal(t, localModelConfig.LocalModelAgentImage, container.Image, "Container image should match config")
		require.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy, "Image pull policy should match config")

		// Check environment variables
		require.Len(t, container.Env, 2, "Should have two env vars")

		// Check container security context
		require.NotNil(t, container.SecurityContext, "Container security context should be set")
		require.False(t, *container.SecurityContext.Privileged, "Container should not be privileged")
		require.NotNil(t, container.SecurityContext.Capabilities, "Container capabilities should be set")
		require.Contains(t, container.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"), "All capabilities should be dropped")
		require.False(t, *container.SecurityContext.AllowPrivilegeEscalation, "Privilege escalation should not be allowed")
		require.True(t, *container.SecurityContext.RunAsNonRoot, "Container should run as non-root")
		require.True(t, *container.SecurityContext.ReadOnlyRootFilesystem, "Root filesystem should be read-only")

		// Check resource requests and limits
		require.Equal(t, resource.MustParse(localModelConfig.LocalModelAgentCpuRequest),
			container.Resources.Requests[corev1.ResourceCPU], "CPU request should match config")
		require.Equal(t, resource.MustParse(localModelConfig.LocalModelAgentMemoryRequest),
			container.Resources.Requests[corev1.ResourceMemory], "Memory request should match config")
		require.Equal(t, resource.MustParse(localModelConfig.LocalModelAgentCpuLimit),
			container.Resources.Limits[corev1.ResourceCPU], "CPU limit should match config")
		require.Equal(t, resource.MustParse(localModelConfig.LocalModelAgentMemoryLimit),
			container.Resources.Limits[corev1.ResourceMemory], "Memory limit should match config")

		// Check volume mounts
		require.Len(t, container.VolumeMounts, 1, "Should have one volume mount")
		require.Equal(t, "models", container.VolumeMounts[0].Name, "Volume mount name should be 'models'")
		require.Equal(t, "/mnt/models", container.VolumeMounts[0].MountPath, "Mount path should be correct")
		require.False(t, container.VolumeMounts[0].ReadOnly, "Volume should not be read-only")
	})

	t.Run("validate daemonset with different configuration", func(t *testing.T) {
		// Create a different config
		differentConfig := v1beta1.LocalModelConfig{
			LocalModelAgentImage:           "different/image:v1",
			LocalModelAgentImagePullPolicy: "Always",
			LocalModelAgentCpuRequest:      "200m",
			LocalModelAgentMemoryRequest:   "400Mi",
			LocalModelAgentCpuLimit:        "1",
			LocalModelAgentMemoryLimit:     "1Gi",
		}

		// Create the daemonset with different config
		ds := createLocalModelAgentDaemonSet(nodeGroup, differentConfig, "different-pvc")

		// Check that the configuration is applied correctly
		container := ds.Spec.Template.Spec.Containers[0]
		require.Equal(t, differentConfig.LocalModelAgentImage, container.Image, "Container image should match different config")
		require.Equal(t, corev1.PullAlways, container.ImagePullPolicy, "Image pull policy should match different config")

		// Check resource values
		require.Equal(t, resource.MustParse(differentConfig.LocalModelAgentCpuRequest),
			container.Resources.Requests[corev1.ResourceCPU], "CPU request should match different config")
		require.Equal(t, resource.MustParse(differentConfig.LocalModelAgentMemoryRequest),
			container.Resources.Requests[corev1.ResourceMemory], "Memory request should match different config")

		// Check PVC name
		require.Equal(t, "different-pvc",
			ds.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName,
			"PVC name should match different name")
	})
}

func TestReconcileDaemonSet(t *testing.T) {
	// Setup common test resources
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add appsv1 to scheme: %v", err)
	}
	// Create a test LocalModelNodeGroup
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-group",
		},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/hostname",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create test config
	localModelConfig := &v1beta1.LocalModelConfig{
		LocalModelAgentImage:           "kserve/agent:latest",
		LocalModelAgentImagePullPolicy: "IfNotPresent",
		LocalModelAgentCpuRequest:      "100m",
		LocalModelAgentMemoryRequest:   "200Mi",
		LocalModelAgentCpuLimit:        "500m",
		LocalModelAgentMemoryLimit:     "500Mi",
	}

	pvcName := "test-pvc-name"
	expectedDaemonSetName := "test-node-group-agent"

	t.Run("successfully create DaemonSet when it doesn't exist", func(t *testing.T) {
		client := fakeclient.NewClientBuilder().WithScheme(scheme).Build()

		// Create the reconciler
		reconciler := &LocalModelNodeGroupReconciler{
			Client: client,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, pvcName)

		// Check results
		require.NoError(t, err, "Expected no error when creating DaemonSet")

		// Verify the DaemonSet was created
		daemonset := &appsv1.DaemonSet{}
		err = client.Get(t.Context(), types.NamespacedName{Name: expectedDaemonSetName, Namespace: constants.KServeNamespace}, daemonset)
		require.NoError(t, err, "Expected to find the created DaemonSet")
		require.Equal(t, expectedDaemonSetName, daemonset.Name, "Expected DaemonSet name to match")
	})

	t.Run("update DaemonSet when it already exists with different spec", func(t *testing.T) {
		// Create an existing DaemonSet with different spec
		existingDS := createLocalModelAgentDaemonSet(*nodeGroup, *localModelConfig, "old-pvc-name")
		existingDS.Namespace = constants.KServeNamespace

		client := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingDS).Build()

		// Create the reconciler
		reconciler := &LocalModelNodeGroupReconciler{
			Client: client,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function with new PVC name
		newPvcName := "new-pvc-name"
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, newPvcName)

		// Check results
		require.NoError(t, err, "Expected no error when updating DaemonSet")

		// Verify the DaemonSet was updated with the new PVC name
		updatedDS := &appsv1.DaemonSet{}
		err = client.Get(t.Context(), types.NamespacedName{Name: expectedDaemonSetName, Namespace: constants.KServeNamespace}, updatedDS)
		require.NoError(t, err, "Expected to find the updated DaemonSet")

		// Check that the volume mount now references the new PVC name
		updatedPvcName := updatedDS.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName
		require.Equal(t, newPvcName, updatedPvcName, "Expected PVC name to be updated in the DaemonSet")
	})

	t.Run("handle error when setting controller reference fails", func(t *testing.T) {
		// Setup a fake client
		client := fakeclient.NewClientBuilder().WithScheme(scheme).Build()

		// Create a reconciler with a bad scheme that will cause SetControllerReference to fail
		badScheme := runtime.NewScheme() // Intentionally don't register types
		reconciler := &LocalModelNodeGroupReconciler{
			Client: client,
			Log:    logr.Discard(),
			Scheme: badScheme,
		}

		// Call the function
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, pvcName)

		// Check results
		require.Error(t, err, "Expected error when setting controller reference fails")

		// Verify no DaemonSet was created
		daemonset := &appsv1.DaemonSet{}
		err = client.Get(t.Context(), types.NamespacedName{Name: expectedDaemonSetName, Namespace: constants.KServeNamespace}, daemonset)
		require.Error(t, err, "Expected not to find a DaemonSet")
		require.True(t, k8serrors.IsNotFound(err), "Expected not found error")
	})

	t.Run("handle error when creating DaemonSet fails", func(t *testing.T) {
		// Create a mock client that will fail the Create operation
		mockClient := &mockFailClient{
			Client:      fakeclient.NewClientBuilder().WithScheme(scheme).Build(),
			failCreate:  true,
			failGet:     false,
			failUpdate:  false,
			returnError: k8serrors.NewInternalError(errors.New("internal server error")),
		}

		// Create the reconciler with mock client
		reconciler := &LocalModelNodeGroupReconciler{
			Client: mockClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, pvcName)

		// Check results
		require.Error(t, err, "Expected error when creating DaemonSet fails")
		require.Contains(t, err.Error(), "internal server error", "Expected error message to indicate creation failure")
	})

	t.Run("handle error when getting DaemonSet fails", func(t *testing.T) {
		// Create a mock client that will fail the Get operation
		mockClient := &mockFailClient{
			Client:      fakeclient.NewClientBuilder().WithScheme(scheme).Build(),
			failCreate:  false,
			failGet:     true,
			failUpdate:  false,
			returnError: k8serrors.NewInternalError(errors.New("internal server error")),
		}

		// Create the reconciler with mock client
		reconciler := &LocalModelNodeGroupReconciler{
			Client: mockClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, pvcName)

		// Check results
		require.Error(t, err, "Expected error when getting DaemonSet fails")
		require.Contains(t, err.Error(), "internal server error", "Expected error message to indicate get failure")
	})

	t.Run("handle error when updating DaemonSet fails", func(t *testing.T) {
		// Create an existing DaemonSet
		existingDS := createLocalModelAgentDaemonSet(*nodeGroup, *localModelConfig, "old-pvc-name")
		existingDS.Namespace = constants.KServeNamespace

		// Create a mock client that will fail the Update operation but allow Get and return the existing DS
		mockClient := &mockFailClient{
			Client:      fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingDS).Build(),
			failCreate:  false,
			failGet:     false,
			failUpdate:  true,
			returnError: k8serrors.NewInternalError(errors.New("internal server error")),
		}

		// Create the reconciler with mock client
		reconciler := &LocalModelNodeGroupReconciler{
			Client: mockClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function with different PVC name to trigger update
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, "new-pvc-name")

		// Check results
		require.Error(t, err, "Expected error when updating DaemonSet fails")
		require.Contains(t, err.Error(), "internal server error", "Expected error message to indicate update failure")
	})

	t.Run("no update when DaemonSet hasn't changed", func(t *testing.T) {
		// Create an existing DaemonSet with the exact same spec that would be created
		existingDS := createLocalModelAgentDaemonSet(*nodeGroup, *localModelConfig, pvcName)
		existingDS.Namespace = constants.KServeNamespace

		// Setup a spy client to detect update calls
		spyClient := &spyClient{
			Client:       fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingDS).Build(),
			updateCalled: false,
		}

		// Create the reconciler with the spy client
		reconciler := &LocalModelNodeGroupReconciler{
			Client: spyClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		// Call the function with the same PVC name, which shouldn't trigger update
		err := reconciler.reconcileDaemonSet(t.Context(), nodeGroup, localModelConfig, pvcName)

		// Check results
		require.NoError(t, err, "Expected no error")
		// Verify update wasn't called (except for the dry run)
		require.False(t, spyClient.updateCalled, "Expected no update to be performed")
	})
}

// mockFailClient is a mock client that can be configured to fail specific operations
type mockFailClient struct {
	client.Client
	failCreate  bool
	failGet     bool
	failUpdate  bool
	failPatch   bool
	returnError error
}

func (m *mockFailClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.failCreate {
		return m.returnError
	}
	return m.Client.Create(ctx, obj, opts...)
}

func (m *mockFailClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.failGet {
		return m.returnError
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

func (m *mockFailClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Check if this is a dry run
	isDryRun := false
	for _, opt := range opts {
		if opt == client.DryRunAll {
			isDryRun = true
			break
		}
	}

	if m.failUpdate && !isDryRun {
		return m.returnError
	}
	return m.Client.Update(ctx, obj, opts...)
}

func (m *mockFailClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if m.failPatch {
		return m.returnError
	}
	return m.Client.Patch(ctx, obj, patch, opts...)
}

// spyClient tracks if Update was called
type spyClient struct {
	client.Client
	updateCalled bool
}

func (s *spyClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Check if this is a dry run
	isDryRun := false
	for _, opt := range opts {
		if opt == client.DryRunAll {
			isDryRun = true
			break
		}
	}

	if !isDryRun {
		s.updateCalled = true
	}
	return s.Client.Update(ctx, obj, opts...)
}
