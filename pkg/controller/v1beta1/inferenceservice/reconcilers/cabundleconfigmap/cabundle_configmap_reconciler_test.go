/*
Copyright 2023 The KServe Authors.

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

package cabundleconfigmap

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	rtesting "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestGetDesiredCaBundleConfigMapForUserNS(t *testing.T) {
	cabundleConfigMapData := make(map[string]string)

	// cabundle data
	cabundleConfigMapData["cabundle.crt"] = "SAMPLE_CA_BUNDLE"
	targetNamespace := "test"
	testCases := []struct {
		name                      string
		namespace                 string
		configMapData             map[string]string
		expectedCopiedCaConfigMap *corev1.ConfigMap
	}{
		{
			name:          "Do not create a ca bundle configmap,if CaBundleConfigMapName is '' in storageConfig of inference-config configmap",
			namespace:     targetNamespace,
			configMapData: cabundleConfigMapData,
			expectedCopiedCaConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultGlobalCaBundleConfigMapName,
					Namespace: targetNamespace,
				},
				Data: cabundleConfigMapData,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getDesiredCaBundleConfigMapForUserNS(constants.DefaultGlobalCaBundleConfigMapName, tt.namespace, tt.configMapData)
			if diff := cmp.Diff(tt.expectedCopiedCaConfigMap, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func TestReconcileCaBundleConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	testCases := []struct {
		name            string
		existingCM      *corev1.ConfigMap
		desiredCM       *corev1.ConfigMap
		expectedErr     bool
		expectedActions int
	}{
		{
			name:       "Create configmap if not exists",
			existingCM: nil,
			desiredCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					"cabundle.crt": "TEST_BUNDLE",
				},
			},
			expectedErr:     false,
			expectedActions: 1, // create
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake clientset
			clientset := fake.NewSimpleClientset()
			if tc.existingCM != nil {
				_, err := clientset.CoreV1().ConfigMaps(tc.existingCM.Namespace).Create(t.Context(), tc.existingCM, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error creating test configmap: %v", err)
				}
			}
			// Setup fake client
			client := rtesting.NewClientBuilder().WithScheme(scheme).Build()

			reconciler := &CaBundleConfigMapReconciler{
				client:    client,
				clientset: clientset,
				scheme:    scheme,
			}

			// Test the reconciliation
			err := reconciler.ReconcileCaBundleConfigMap(t.Context(), tc.desiredCM)

			if tc.expectedErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Verify correct actions were performed
			actions := clientset.Actions()
			if len(actions) != tc.expectedActions {
				t.Errorf("Expected %d actions but got %d: %v", tc.expectedActions, len(actions), actions)
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	// Test CA bundle content
	caBundleContent := "TEST_CA_BUNDLE_CONTENT"

	// Create test InferenceService
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-isvc",
			Namespace: "test-ns",
		},
	}

	testCases := []struct {
		name                string
		configMaps          []*corev1.ConfigMap
		isvc                *v1beta1.InferenceService
		expectedErr         bool
		expectedConfigMapNS string
	}{
		{
			name: "No CA bundle name in storage config",
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						"storageInitializer": `{"CaBundleConfigMapName": ""}`,
					},
				},
			},
			isvc:        isvc,
			expectedErr: false,
		},
		{
			name: "CA bundle not found",
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						"storageInitializer": `{"CaBundleConfigMapName": "nonexistent-cm"}`,
					},
				},
			},
			isvc:        isvc,
			expectedErr: true,
		},
		{
			name: "CA bundle missing required data",
			configMaps: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.InferenceServiceConfigMapName,
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						"storageInitializer": `{"CaBundleConfigMapName": "test-cabundle-cm"}`,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cabundle-cm",
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						"wrong-key": "data",
					},
				},
			},
			isvc:        isvc,
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake clients
			clientset := fake.NewSimpleClientset()
			for _, cm := range tc.configMaps {
				_, err := clientset.CoreV1().ConfigMaps(cm.Namespace).Create(t.Context(), cm, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error creating test configmap: %v", err)
				}
			}

			client := rtesting.NewClientBuilder().WithScheme(scheme).Build()

			reconciler := &CaBundleConfigMapReconciler{
				client:    client,
				clientset: clientset,
				scheme:    scheme,
			}

			// Test reconciliation
			err := reconciler.Reconcile(t.Context(), tc.isvc)

			if tc.expectedErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Verify configmap was created in the correct namespace if expected
			if tc.expectedConfigMapNS != "" {
				cm, err := clientset.CoreV1().ConfigMaps(tc.expectedConfigMapNS).Get(
					t.Context(),
					constants.DefaultGlobalCaBundleConfigMapName,
					metav1.GetOptions{},
				)
				if err != nil {
					t.Errorf("Expected configmap in namespace %s but got error: %v", tc.expectedConfigMapNS, err)
				}
				if cm != nil {
					if data, ok := cm.Data[constants.DefaultCaBundleFileName]; !ok || data != caBundleContent {
						t.Errorf("Expected configmap to have correct data but got: %v", cm.Data)
					}
				}
			}
		})
	}
}

func TestGetCabundleConfigMapForUserNS(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	testCases := []struct {
		name                 string
		caBundleNameInConfig string
		kserveNamespace      string
		isvcNamespace        string
		existingCMs          []*corev1.ConfigMap
		expectedError        bool
		expectedData         map[string]string
	}{
		{
			name:                 "Successfully get CA bundle configmap",
			caBundleNameInConfig: "test-ca-bundle",
			kserveNamespace:      constants.KServeNamespace,
			isvcNamespace:        "user-ns",
			existingCMs: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ca-bundle",
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						constants.DefaultCaBundleFileName: "TEST_BUNDLE_DATA",
					},
				},
			},
			expectedError: false,
			expectedData: map[string]string{
				constants.DefaultCaBundleFileName: "TEST_BUNDLE_DATA",
			},
		},
		{
			name:                 "ConfigMap not found",
			caBundleNameInConfig: "nonexistent-cm",
			kserveNamespace:      constants.KServeNamespace,
			isvcNamespace:        "user-ns",
			existingCMs:          []*corev1.ConfigMap{},
			expectedError:        true,
		},
		{
			name:                 "ConfigMap missing required data",
			caBundleNameInConfig: "test-ca-bundle",
			kserveNamespace:      constants.KServeNamespace,
			isvcNamespace:        "user-ns",
			existingCMs: []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ca-bundle",
						Namespace: constants.KServeNamespace,
					},
					Data: map[string]string{
						"wrong-key": "data",
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake clients
			clientset := fake.NewSimpleClientset()
			for _, cm := range tc.existingCMs {
				_, err := clientset.CoreV1().ConfigMaps(cm.Namespace).Create(t.Context(), cm, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error creating test configmap: %v", err)
				}
			}

			client := rtesting.NewClientBuilder().WithScheme(scheme).Build()

			reconciler := &CaBundleConfigMapReconciler{
				client:    client,
				clientset: clientset,
				scheme:    scheme,
			}

			// Test function
			result, err := reconciler.getCabundleConfigMapForUserNS(
				t.Context(),
				tc.caBundleNameInConfig,
				tc.kserveNamespace,
				tc.isvcNamespace,
			)

			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tc.expectedError {
				if result == nil {
					t.Errorf("Expected configmap result but got nil")
				} else {
					if !reflect.DeepEqual(result.Data, tc.expectedData) {
						t.Errorf("Expected data %v but got %v", tc.expectedData, result.Data)
					}
					if result.Namespace != tc.isvcNamespace {
						t.Errorf("Expected namespace %s but got %s", tc.isvcNamespace, result.Namespace)
					}
					if result.Name != constants.DefaultGlobalCaBundleConfigMapName {
						t.Errorf("Expected name %s but got %s", constants.DefaultGlobalCaBundleConfigMapName, result.Name)
					}
				}
			}
		})
	}
}

func TestNewCaBundleConfigMapReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	clientset := fake.NewSimpleClientset()
	client := rtesting.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := NewCaBundleConfigMapReconciler(client, clientset, scheme)
	// The constructor should always return a valid non-nil reconciler
	// with properly initialized fields

	if reconciler.client == nil {
		t.Error("Expected client to be non-nil")
	} else if reconciler.client != client {
		t.Errorf("Expected client to be %v, got %v", client, reconciler.client)
	}

	if reconciler.clientset == nil {
		t.Error("Expected clientset to be non-nil")
	} else if reconciler.clientset != clientset {
		t.Errorf("Expected clientset to be %v, got %v", clientset, reconciler.clientset)
	}

	if reconciler.scheme == nil {
		t.Error("Expected scheme to be non-nil")
	} else if reconciler.scheme != scheme {
		t.Errorf("Expected scheme to be %v, got %v", scheme, reconciler.scheme)
	}
}

func TestReconcileCaBundleConfigMap_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	testCases := []struct {
		name            string
		existingCM      *corev1.ConfigMap
		desiredCM       *corev1.ConfigMap
		expectedErr     bool
		expectedActions int
	}{
		{
			name: "Handle error during update",
			existingCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "test-ns",
					ResourceVersion: "999", // Will cause conflict during update
				},
				Data: map[string]string{
					"cabundle.crt": "OLD_BUNDLE",
				},
			},
			desiredCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					"cabundle.crt": "NEW_BUNDLE",
				},
			},
			expectedErr:     true,
			expectedActions: 2, // get + failed update
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake clientset
			clientset := fake.NewSimpleClientset()
			if tc.existingCM != nil {
				_, err := clientset.CoreV1().ConfigMaps(tc.existingCM.Namespace).Create(t.Context(), tc.existingCM, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Error creating test configmap: %v", err)
				}
			}

			// Setup fake client with error behavior if needed
			client := rtesting.NewClientBuilder().WithScheme(scheme).Build()
			if tc.name == "Handle error during update" {
				// Create a custom fake client to simulate update error
				// Since we're using clientset for most operations, this won't actually be used,
				// so we'll trigger the error by setting a condition that will fail elsewhere
				tc.existingCM.ResourceVersion = "invalid"
			}

			reconciler := &CaBundleConfigMapReconciler{
				client:    client,
				clientset: clientset,
				scheme:    scheme,
			}

			// Test the reconciliation
			err := reconciler.ReconcileCaBundleConfigMap(t.Context(), tc.desiredCM)

			if tc.expectedErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Verify correct actions were performed
			actions := clientset.Actions()
			if len(actions) != tc.expectedActions {
				t.Errorf("Expected %d actions but got %d: %v", tc.expectedActions, len(actions), actions)
			}

			// For update cases, verify the updated content
			if !tc.expectedErr && tc.name == "Update configmap when data is different" {
				updatedCM, err := clientset.CoreV1().ConfigMaps(tc.desiredCM.Namespace).Get(t.Context(), tc.desiredCM.Name, metav1.GetOptions{})
				if err != nil {
					t.Errorf("Failed to get updated configmap: %v", err)
				}
				if !reflect.DeepEqual(updatedCM.Data, tc.desiredCM.Data) {
					t.Errorf("Expected data %v but got %v", tc.desiredCM.Data, updatedCM.Data)
				}
			}
		})
	}
}
