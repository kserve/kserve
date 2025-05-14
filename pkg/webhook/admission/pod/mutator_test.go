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

package pod

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/onsi/gomega"
	gomegaTypes "github.com/onsi/gomega/types"
	"golang.org/x/net/context"
	"gomodules.xyz/jsonpatch/v2"
	"google.golang.org/protobuf/proto"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/constants"
)

func TestMutator_Handle(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	kserveNamespace := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.KServeNamespace,
		},
		Spec:   corev1.NamespaceSpec{},
		Status: corev1.NamespaceStatus{},
	}

	if err := c.Create(context.Background(), &kserveNamespace); err != nil {
		t.Errorf("failed to create namespace: %v", err)
	}
	mutator := Mutator{Client: c, Clientset: clientset, Decoder: admission.NewDecoder(c.Scheme())}

	cases := map[string]struct {
		configMap corev1.ConfigMap
		request   admission.Request
		pod       corev1.Pod
		matcher   gomegaTypes.GomegaMatcher
	}{
		"should not mutate non isvc pods": {
			configMap: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Immutable: nil,
				Data: map[string]string{
					StorageInitializerConfigMapKeyName: `{
						"image" : "kserve/storage-initializer:latest",
						"memoryRequest": "100Mi",
						"memoryLimit": "1Gi",
						"cpuRequest": "100m",
						"cpuLimit": "1",
						"storageSpecSecretName": "storage-config"
					}`,
					LoggerConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "100Mi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "100m",
        				"cpuLimit": "1",
        				"defaultUrl": "http://default-broker"
    				}`,
					BatcherConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "1Gi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "1",
        				"cpuLimit": "1"
    				}`,
					constants.AgentConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "100Mi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "100m",
        				"cpuLimit": "1"
    				}`,
				},
				BinaryData: nil,
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID(uuid.NewString()),
					Kind: metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					Resource: metav1.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					SubResource: "",
					RequestKind: &metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					RequestResource: &metav1.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					RequestSubResource: "",
					Name:               "",
					Namespace:          "default",
					Operation:          admissionv1.Create,
					Object:             runtime.RawExtension{},
					OldObject:          runtime.RawExtension{},
					DryRun:             nil,
					Options:            runtime.RawExtension{},
				},
			},
			pod: corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
			},
			matcher: gomega.Equal(admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					UID:     "",
					Allowed: true,
					Result: &metav1.Status{
						TypeMeta: metav1.TypeMeta{},
						ListMeta: metav1.ListMeta{},
						Status:   "",
						Message:  "",
						Reason:   "",
						Details:  nil,
						Code:     200,
					},
					Patch:            nil,
					PatchType:        nil,
					AuditAnnotations: nil,
					Warnings:         nil,
				},
			}),
		},
		"should mutate isvc pods": {
			configMap: corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Immutable: nil,
				Data: map[string]string{
					StorageInitializerConfigMapKeyName: `{
						"image" : "kserve/storage-initializer:latest",
						"memoryRequest": "100Mi",
						"memoryLimit": "1Gi",
						"cpuRequest": "100m",
						"cpuLimit": "1",
						"storageSpecSecretName": "storage-config"
					}`,
					LoggerConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "100Mi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "100m",
        				"cpuLimit": "1",
        				"defaultUrl": "http://default-broker"
    				}`,
					BatcherConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "1Gi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "1",
        				"cpuLimit": "1"
    				}`,
					constants.AgentConfigMapKeyName: `{
        				"image" : "kserve/agent:latest",
        				"memoryRequest": "100Mi",
        				"memoryLimit": "1Gi",
        				"cpuRequest": "100m",
        				"cpuLimit": "1"
    				}`,
				},
				BinaryData: nil,
			},
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID(uuid.NewString()),
					Kind: metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					Resource: metav1.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					SubResource: "",
					RequestKind: &metav1.GroupVersionKind{
						Group:   "",
						Version: "v1",
						Kind:    "Pod",
					},
					RequestResource: &metav1.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
					RequestSubResource: "",
					Name:               "",
					Namespace:          "default",
					Operation:          admissionv1.Create,
					Object:             runtime.RawExtension{},
					OldObject:          runtime.RawExtension{},
					DryRun:             nil,
					Options:            runtime.RawExtension{},
				},
			},
			pod: corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.InferenceServicePodLabelKey: "",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			matcher: gomega.BeEquivalentTo(admission.Response{
				Patches: []jsonpatch.JsonPatchOperation{
					{
						Operation: "add",
						Path:      "/metadata/annotations",
						Value: map[string]interface{}{
							"serving.kserve.io/enable-metric-aggregation":  "",
							"serving.kserve.io/enable-prometheus-scraping": "",
						},
					},
					{
						Operation: "add",
						Path:      "/metadata/namespace",
						Value:     "default",
					},
				},
				AdmissionResponse: admissionv1.AdmissionResponse{
					UID:              "",
					Allowed:          true,
					Result:           nil,
					Patch:            nil,
					PatchType:        (*admissionv1.PatchType)(proto.String(string(admissionv1.PatchTypeJSONPatch))),
					AuditAnnotations: nil,
					Warnings:         nil,
				},
			}),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := c.Create(context.Background(), &tc.configMap); err != nil {
				t.Errorf("failed to create config map: %v", err)
			}
			byteData, err := json.Marshal(tc.pod)
			if err != nil {
				t.Errorf("failed to marshal pod data: %v", err)
			}
			tc.request.Object.Raw = byteData
			res := mutator.Handle(context.Background(), tc.request)
			sortPatches(res.Patches)
			g.Expect(res).Should(tc.matcher)
			if err := c.Delete(context.Background(), &tc.configMap); err != nil {
				t.Errorf("failed to delete configmap %v", err)
			}
		})
	}
}

// sortPatches sorts the slice of patches by Path so that the comparison works
// when there are > 1 patches. Note: make sure the matcher Patches are sorted.
func sortPatches(patches []jsonpatch.JsonPatchOperation) {
	if len(patches) > 1 {
		sort.Slice(patches, func(i, j int) bool {
			return patches[i].Path < patches[j].Path
		})
	}
}
