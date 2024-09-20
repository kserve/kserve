//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by defaulter-gen. DO NOT EDIT.

package v1beta1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// RegisterDefaults adds defaulters functions to the given scheme.
// Public to allow building arbitrary schemes.
// All generated defaulters are covering - they call all nested defaulters.
func RegisterDefaults(scheme *runtime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&InferenceService{}, func(obj interface{}) { SetObjectDefaults_InferenceService(obj.(*InferenceService)) })
	scheme.AddTypeDefaultingFunc(&InferenceServiceList{}, func(obj interface{}) { SetObjectDefaults_InferenceServiceList(obj.(*InferenceServiceList)) })
	return nil
}

func SetObjectDefaults_InferenceService(in *InferenceService) {
	if in.Spec.Predictor.SKLearn != nil {
		for i := range in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.SKLearn.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.XGBoost != nil {
		for i := range in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.XGBoost.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.Tensorflow != nil {
		for i := range in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Tensorflow.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.PyTorch != nil {
		for i := range in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PyTorch.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.Triton != nil {
		for i := range in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Triton.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.ONNX != nil {
		for i := range in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.ONNX.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.HuggingFace != nil {
		for i := range in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.HuggingFace.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.PMML != nil {
		for i := range in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.PMML.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.LightGBM != nil {
		for i := range in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.LightGBM.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.Paddle != nil {
		for i := range in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Paddle.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.Model != nil {
		for i := range in.Spec.Predictor.Model.PredictorExtensionSpec.Container.Ports {
			a := &in.Spec.Predictor.Model.PredictorExtensionSpec.Container.Ports[i]
			if a.Protocol == "" {
				a.Protocol = "TCP"
			}
		}
		if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.LivenessProbe != nil {
			if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Model.PredictorExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.ReadinessProbe != nil {
			if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Model.PredictorExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.StartupProbe != nil {
			if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
				if in.Spec.Predictor.Model.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					in.Spec.Predictor.Model.PredictorExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Predictor.WorkerSpec != nil {
		for i := range in.Spec.Predictor.WorkerSpec.InitContainers {
			a := &in.Spec.Predictor.WorkerSpec.InitContainers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Predictor.WorkerSpec.Containers {
			a := &in.Spec.Predictor.WorkerSpec.Containers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Predictor.WorkerSpec.EphemeralContainers {
			a := &in.Spec.Predictor.WorkerSpec.EphemeralContainers[i]
			for j := range a.EphemeralContainerCommon.Ports {
				b := &a.EphemeralContainerCommon.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.EphemeralContainerCommon.LivenessProbe != nil {
				if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.ReadinessProbe != nil {
				if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.StartupProbe != nil {
				if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
	}
	for i := range in.Spec.Predictor.PodSpec.InitContainers {
		a := &in.Spec.Predictor.PodSpec.InitContainers[i]
		for j := range a.Ports {
			b := &a.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		if a.LivenessProbe != nil {
			if a.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.ReadinessProbe != nil {
			if a.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.StartupProbe != nil {
			if a.StartupProbe.ProbeHandler.GRPC != nil {
				if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	for i := range in.Spec.Predictor.PodSpec.Containers {
		a := &in.Spec.Predictor.PodSpec.Containers[i]
		for j := range a.Ports {
			b := &a.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		if a.LivenessProbe != nil {
			if a.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.ReadinessProbe != nil {
			if a.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.StartupProbe != nil {
			if a.StartupProbe.ProbeHandler.GRPC != nil {
				if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	for i := range in.Spec.Predictor.PodSpec.EphemeralContainers {
		a := &in.Spec.Predictor.PodSpec.EphemeralContainers[i]
		for j := range a.EphemeralContainerCommon.Ports {
			b := &a.EphemeralContainerCommon.Ports[j]
			if b.Protocol == "" {
				b.Protocol = "TCP"
			}
		}
		if a.EphemeralContainerCommon.LivenessProbe != nil {
			if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.EphemeralContainerCommon.ReadinessProbe != nil {
			if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
		if a.EphemeralContainerCommon.StartupProbe != nil {
			if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC != nil {
				if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service == nil {
					var ptrVar1 string = ""
					a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
				}
			}
		}
	}
	if in.Spec.Explainer != nil {
		if in.Spec.Explainer.ART != nil {
			for i := range in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.Ports {
				a := &in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.Ports[i]
				if a.Protocol == "" {
					a.Protocol = "TCP"
				}
			}
			if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.LivenessProbe != nil {
				if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC != nil {
					if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.ReadinessProbe != nil {
				if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC != nil {
					if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.StartupProbe != nil {
				if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC != nil {
					if in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						in.Spec.Explainer.ART.ExplainerExtensionSpec.Container.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Explainer.PodSpec.InitContainers {
			a := &in.Spec.Explainer.PodSpec.InitContainers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Explainer.PodSpec.Containers {
			a := &in.Spec.Explainer.PodSpec.Containers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Explainer.PodSpec.EphemeralContainers {
			a := &in.Spec.Explainer.PodSpec.EphemeralContainers[i]
			for j := range a.EphemeralContainerCommon.Ports {
				b := &a.EphemeralContainerCommon.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.EphemeralContainerCommon.LivenessProbe != nil {
				if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.ReadinessProbe != nil {
				if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.StartupProbe != nil {
				if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
	}
	if in.Spec.Transformer != nil {
		for i := range in.Spec.Transformer.PodSpec.InitContainers {
			a := &in.Spec.Transformer.PodSpec.InitContainers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Transformer.PodSpec.Containers {
			a := &in.Spec.Transformer.PodSpec.Containers[i]
			for j := range a.Ports {
				b := &a.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.LivenessProbe != nil {
				if a.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.ReadinessProbe != nil {
				if a.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.StartupProbe != nil {
				if a.StartupProbe.ProbeHandler.GRPC != nil {
					if a.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
		for i := range in.Spec.Transformer.PodSpec.EphemeralContainers {
			a := &in.Spec.Transformer.PodSpec.EphemeralContainers[i]
			for j := range a.EphemeralContainerCommon.Ports {
				b := &a.EphemeralContainerCommon.Ports[j]
				if b.Protocol == "" {
					b.Protocol = "TCP"
				}
			}
			if a.EphemeralContainerCommon.LivenessProbe != nil {
				if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.LivenessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.ReadinessProbe != nil {
				if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.ReadinessProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
			if a.EphemeralContainerCommon.StartupProbe != nil {
				if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC != nil {
					if a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service == nil {
						var ptrVar1 string = ""
						a.EphemeralContainerCommon.StartupProbe.ProbeHandler.GRPC.Service = &ptrVar1
					}
				}
			}
		}
	}
}

func SetObjectDefaults_InferenceServiceList(in *InferenceServiceList) {
	for i := range in.Items {
		a := &in.Items[i]
		SetObjectDefaults_InferenceService(a)
	}
}
