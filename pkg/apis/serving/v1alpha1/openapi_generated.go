// +build !ignore_autogenerated

/*
Copyright 2019 kubeflow.org.

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

// Code generated by main. DO NOT EDIT.

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1alpha1

import (
	openapispec "github.com/go-openapi/spec"
	common "k8s.io/kube-openapi/pkg/common"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"github.com/knative/pkg/apis.Condition": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "Conditions defines a readiness condition for a Knative resource. See: https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties",
					Properties: map[string]openapispec.Schema{
						"type": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Type of condition.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"status": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Status of the condition, one of True, False, Unknown.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"severity": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Severity with which to treat failures of this type of condition. When this is not specified, it defaults to Error.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"lastTransitionTime": {
							SchemaProps: openapispec.SchemaProps{
								Description: "LastTransitionTime is the last time the condition transitioned from one status to another. We use VolatileTime in place of metav1.Time to exclude this from creating equality.Semantic differences (all other things held constant).",
								Ref:         ref("github.com/knative/pkg/apis.VolatileTime"),
							},
						},
						"reason": {
							SchemaProps: openapispec.SchemaProps{
								Description: "The reason for the condition's last transition.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"message": {
							SchemaProps: openapispec.SchemaProps{
								Description: "A human readable message indicating details about the transition.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
					},
					Required: []string{"type", "status"},
				},
			},
			Dependencies: []string{
				"github.com/knative/pkg/apis.VolatileTime"},
		},
		"github.com/knative/pkg/apis.VolatileTime": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "VolatileTime wraps metav1.Time",
					Properties: map[string]openapispec.Schema{
						"Inner": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
							},
						},
					},
					Required: []string{"Inner"},
				},
			},
			Dependencies: []string{
				"k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.AlibiExplainSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "AlibiExplainSpec defines the arguments for configuring an Alibi Explanation Server",
					Properties: map[string]openapispec.Schema{
						"type": {
							SchemaProps: openapispec.SchemaProps{
								Description: "The type of Alibi explainer",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"storageUri": {
							SchemaProps: openapispec.SchemaProps{
								Description: "The location of a trained explanation model",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest Alibi Version.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
						"config": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Inline custom parameter settings for explainer",
								Type:        []string{"object"},
								AdditionalProperties: &openapispec.SchemaOrBool{
									Schema: &openapispec.Schema{
										SchemaProps: openapispec.SchemaProps{
											Type:   []string{"string"},
											Format: "",
										},
									},
								},
							},
						},
					},
					Required: []string{"type"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.CustomSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "CustomSpec provides a hook for arbitrary container configuration.",
					Properties: map[string]openapispec.Schema{
						"container": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("k8s.io/api/core/v1.Container"),
							},
						},
					},
					Required: []string{"container"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.Container"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ExplainSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "ExplainSpec defines the arguments for a model explanation server",
					Properties: map[string]openapispec.Schema{
						"alibi": {
							SchemaProps: openapispec.SchemaProps{
								Description: "The following fields follow a \"1-of\" semantic. Users must specify exactly one openapispec.",
								Ref:         ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.AlibiExplainSpec"),
							},
						},
						"custom": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.CustomSpec"),
							},
						},
					},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.AlibiExplainSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.CustomSpec"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Properties: map[string]openapispec.Schema{
						"image": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
					},
					Required: []string{"image"},
				},
			},
			Dependencies: []string{},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworksConfig": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Properties: map[string]openapispec.Schema{
						"tensorflow": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"),
							},
						},
						"tensorrt": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"),
							},
						},
						"xgboost": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"),
							},
						},
						"sklearn": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"),
							},
						},
						"pytorch": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"),
							},
						},
					},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.FrameworkConfig"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFService": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "KFService is the Schema for the services API",
					Properties: map[string]openapispec.Schema{
						"kind": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"apiVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"metadata": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
							},
						},
						"spec": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceSpec"),
							},
						},
						"status": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceStatus"),
							},
						},
					},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceList": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "KFServiceList contains a list of Service",
					Properties: map[string]openapispec.Schema{
						"kind": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"apiVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"metadata": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"),
							},
						},
						"items": {
							SchemaProps: openapispec.SchemaProps{
								Type: []string{"array"},
								Items: &openapispec.SchemaOrArray{
									Schema: &openapispec.Schema{
										SchemaProps: openapispec.SchemaProps{
											Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFService"),
										},
									},
								},
							},
						},
					},
					Required: []string{"items"},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFService", "k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "KFServiceSpec defines the desired state of KFService",
					Properties: map[string]openapispec.Schema{
						"default": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ModelSpec"),
							},
						},
						"canary": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Canary defines an alternate configuration to route a percentage of traffic.",
								Ref:         ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ModelSpec"),
							},
						},
						"canaryTrafficPercent": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"integer"},
								Format: "int32",
							},
						},
					},
					Required: []string{"default"},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ModelSpec"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.KFServiceStatus": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "KFServiceStatus defines the observed state of KFService",
					Properties: map[string]openapispec.Schema{
						"observedGeneration": {
							SchemaProps: openapispec.SchemaProps{
								Description: "ObservedGeneration is the 'Generation' of the Service that was last processed by the controller.",
								Type:        []string{"integer"},
								Format:      "int64",
							},
						},
						"conditions": {
							VendorExtensible: openapispec.VendorExtensible{
								Extensions: openapispec.Extensions{
									"x-kubernetes-patch-merge-key": "type",
									"x-kubernetes-patch-strategy":  "merge",
								},
							},
							SchemaProps: openapispec.SchemaProps{
								Description: "Conditions the latest available observations of a resource's current state.",
								Type:        []string{"array"},
								Items: &openapispec.SchemaOrArray{
									Schema: &openapispec.Schema{
										SchemaProps: openapispec.SchemaProps{
											Ref: ref("github.com/knative/pkg/apis.Condition"),
										},
									},
								},
							},
						},
						"url": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/knative/pkg/apis.URL"),
							},
						},
						"default": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.StatusConfigurationSpec"),
							},
						},
						"canary": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.StatusConfigurationSpec"),
							},
						},
					},
				},
			},
			Dependencies: []string{
				"github.com/knative/pkg/apis.Condition", "github.com/knative/pkg/apis.URL", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.StatusConfigurationSpec"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ModelSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "ModelSpec defines the configuration to route traffic to a predictor.",
					Properties: map[string]openapispec.Schema{
						"serviceAccountName": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"minReplicas": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Minimum number of replicas, pods won't scale down to 0 in case of no traffic",
								Type:        []string{"integer"},
								Format:      "int32",
							},
						},
						"maxReplicas": {
							SchemaProps: openapispec.SchemaProps{
								Description: "This is the up bound for autoscaler to scale to",
								Type:        []string{"integer"},
								Format:      "int32",
							},
						},
						"custom": {
							SchemaProps: openapispec.SchemaProps{
								Description: "The following fields follow a \"1-of\" semantic. Users must specify exactly one openapispec.",
								Ref:         ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.CustomSpec"),
							},
						},
						"tensorflow": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorflowSpec"),
							},
						},
						"tensorrt": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorRTSpec"),
							},
						},
						"xgboost": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.XGBoostSpec"),
							},
						},
						"sklearn": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.SKLearnSpec"),
							},
						},
						"pytorch": {
							SchemaProps: openapispec.SchemaProps{
								Ref: ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.PyTorchSpec"),
							},
						},
						"explain": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Optional Explain specification to add a model explainer next to the chosen predictor. In future v1alpha2 the above model predictors would be moved down a level.",
								Ref:         ref("github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ExplainSpec"),
							},
						},
					},
				},
			},
			Dependencies: []string{
				"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.CustomSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.ExplainSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.PyTorchSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.SKLearnSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorRTSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorflowSpec", "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.XGBoostSpec"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.PyTorchSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "PyTorchSpec defines arguments for configuring PyTorch model serving.",
					Properties: map[string]openapispec.Schema{
						"modelUri": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"modelClassName": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults PyTorch model class name to 'PyTorchModel'",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest PyTorch Version",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
					},
					Required: []string{"modelUri"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.SKLearnSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "SKLearnSpec defines arguments for configuring SKLearn model serving.",
					Properties: map[string]openapispec.Schema{
						"modelUri": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest SKLearn Version.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
					},
					Required: []string{"modelUri"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.StatusConfigurationSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "StatusConfigurationSpec describes the state of the configuration receiving traffic.",
					Properties: map[string]openapispec.Schema{
						"name": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"replicas": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"integer"},
								Format: "int32",
							},
						},
						"traffic": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"integer"},
								Format: "int32",
							},
						},
					},
				},
			},
			Dependencies: []string{},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorRTSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "TensorRTSpec defines arguments for configuring TensorRT model serving.",
					Properties: map[string]openapispec.Schema{
						"modelUri": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest TensorRT Version.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
					},
					Required: []string{"modelUri"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.TensorflowSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "TensorflowSpec defines arguments for configuring Tensorflow model serving.",
					Properties: map[string]openapispec.Schema{
						"modelUri": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest TF Version.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
					},
					Required: []string{"modelUri"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
		"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1.XGBoostSpec": {
			Schema: openapispec.Schema{
				SchemaProps: openapispec.SchemaProps{
					Description: "XGBoostSpec defines arguments for configuring XGBoost model serving.",
					Properties: map[string]openapispec.Schema{
						"modelUri": {
							SchemaProps: openapispec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"runtimeVersion": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to latest XGBoost Version.",
								Type:        []string{"string"},
								Format:      "",
							},
						},
						"resources": {
							SchemaProps: openapispec.SchemaProps{
								Description: "Defaults to requests and limits of 1CPU, 2Gb MEM.",
								Ref:         ref("k8s.io/api/core/v1.ResourceRequirements"),
							},
						},
					},
					Required: []string{"modelUri"},
				},
			},
			Dependencies: []string{
				"k8s.io/api/core/v1.ResourceRequirements"},
		},
	}
}
