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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AIXExplainerSpec) DeepCopyInto(out *AIXExplainerSpec) {
	*out = *in
	in.ExplainerExtensionSpec.DeepCopyInto(&out.ExplainerExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AIXExplainerSpec.
func (in *AIXExplainerSpec) DeepCopy() *AIXExplainerSpec {
	if in == nil {
		return nil
	}
	out := new(AIXExplainerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ARTExplainerSpec) DeepCopyInto(out *ARTExplainerSpec) {
	*out = *in
	in.ExplainerExtensionSpec.DeepCopyInto(&out.ExplainerExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ARTExplainerSpec.
func (in *ARTExplainerSpec) DeepCopy() *ARTExplainerSpec {
	if in == nil {
		return nil
	}
	out := new(ARTExplainerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AlibiExplainerSpec) DeepCopyInto(out *AlibiExplainerSpec) {
	*out = *in
	in.ExplainerExtensionSpec.DeepCopyInto(&out.ExplainerExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AlibiExplainerSpec.
func (in *AlibiExplainerSpec) DeepCopy() *AlibiExplainerSpec {
	if in == nil {
		return nil
	}
	out := new(AlibiExplainerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Batcher) DeepCopyInto(out *Batcher) {
	*out = *in
	if in.MaxBatchSize != nil {
		in, out := &in.MaxBatchSize, &out.MaxBatchSize
		*out = new(int)
		**out = **in
	}
	if in.MaxLatency != nil {
		in, out := &in.MaxLatency, &out.MaxLatency
		*out = new(int)
		**out = **in
	}
	if in.Timeout != nil {
		in, out := &in.Timeout, &out.Timeout
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Batcher.
func (in *Batcher) DeepCopy() *Batcher {
	if in == nil {
		return nil
	}
	out := new(Batcher)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ComponentExtensionSpec) DeepCopyInto(out *ComponentExtensionSpec) {
	*out = *in
	if in.MinReplicas != nil {
		in, out := &in.MinReplicas, &out.MinReplicas
		*out = new(int)
		**out = **in
	}
	if in.ScaleTarget != nil {
		in, out := &in.ScaleTarget, &out.ScaleTarget
		*out = new(int)
		**out = **in
	}
	if in.ScaleMetric != nil {
		in, out := &in.ScaleMetric, &out.ScaleMetric
		*out = new(ScaleMetric)
		**out = **in
	}
	if in.ContainerConcurrency != nil {
		in, out := &in.ContainerConcurrency, &out.ContainerConcurrency
		*out = new(int64)
		**out = **in
	}
	if in.TimeoutSeconds != nil {
		in, out := &in.TimeoutSeconds, &out.TimeoutSeconds
		*out = new(int64)
		**out = **in
	}
	if in.CanaryTrafficPercent != nil {
		in, out := &in.CanaryTrafficPercent, &out.CanaryTrafficPercent
		*out = new(int64)
		**out = **in
	}
	if in.Logger != nil {
		in, out := &in.Logger, &out.Logger
		*out = new(LoggerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Batcher != nil {
		in, out := &in.Batcher, &out.Batcher
		*out = new(Batcher)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ComponentExtensionSpec.
func (in *ComponentExtensionSpec) DeepCopy() *ComponentExtensionSpec {
	if in == nil {
		return nil
	}
	out := new(ComponentExtensionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ComponentStatusSpec) DeepCopyInto(out *ComponentStatusSpec) {
	*out = *in
	if in.Traffic != nil {
		in, out := &in.Traffic, &out.Traffic
		*out = make([]servingv1.TrafficTarget, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
	if in.RestURL != nil {
		in, out := &in.RestURL, &out.RestURL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
	if in.GrpcURL != nil {
		in, out := &in.GrpcURL, &out.GrpcURL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
	if in.Address != nil {
		in, out := &in.Address, &out.Address
		*out = new(v1.Addressable)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ComponentStatusSpec.
func (in *ComponentStatusSpec) DeepCopy() *ComponentStatusSpec {
	if in == nil {
		return nil
	}
	out := new(ComponentStatusSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomExplainer) DeepCopyInto(out *CustomExplainer) {
	*out = *in
	in.PodSpec.DeepCopyInto(&out.PodSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomExplainer.
func (in *CustomExplainer) DeepCopy() *CustomExplainer {
	if in == nil {
		return nil
	}
	out := new(CustomExplainer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomPredictor) DeepCopyInto(out *CustomPredictor) {
	*out = *in
	in.PodSpec.DeepCopyInto(&out.PodSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomPredictor.
func (in *CustomPredictor) DeepCopy() *CustomPredictor {
	if in == nil {
		return nil
	}
	out := new(CustomPredictor)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomTransformer) DeepCopyInto(out *CustomTransformer) {
	*out = *in
	in.PodSpec.DeepCopyInto(&out.PodSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomTransformer.
func (in *CustomTransformer) DeepCopy() *CustomTransformer {
	if in == nil {
		return nil
	}
	out := new(CustomTransformer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExplainerExtensionSpec) DeepCopyInto(out *ExplainerExtensionSpec) {
	*out = *in
	if in.RuntimeVersion != nil {
		in, out := &in.RuntimeVersion, &out.RuntimeVersion
		*out = new(string)
		**out = **in
	}
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Container.DeepCopyInto(&out.Container)
	if in.Storage != nil {
		in, out := &in.Storage, &out.Storage
		*out = new(StorageSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExplainerExtensionSpec.
func (in *ExplainerExtensionSpec) DeepCopy() *ExplainerExtensionSpec {
	if in == nil {
		return nil
	}
	out := new(ExplainerExtensionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExplainerSpec) DeepCopyInto(out *ExplainerSpec) {
	*out = *in
	if in.Alibi != nil {
		in, out := &in.Alibi, &out.Alibi
		*out = new(AlibiExplainerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.AIX != nil {
		in, out := &in.AIX, &out.AIX
		*out = new(AIXExplainerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.ART != nil {
		in, out := &in.ART, &out.ART
		*out = new(ARTExplainerSpec)
		(*in).DeepCopyInto(*out)
	}
	in.PodSpec.DeepCopyInto(&out.PodSpec)
	in.ComponentExtensionSpec.DeepCopyInto(&out.ComponentExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExplainerSpec.
func (in *ExplainerSpec) DeepCopy() *ExplainerSpec {
	if in == nil {
		return nil
	}
	out := new(ExplainerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FailureInfo) DeepCopyInto(out *FailureInfo) {
	*out = *in
	if in.Time != nil {
		in, out := &in.Time, &out.Time
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FailureInfo.
func (in *FailureInfo) DeepCopy() *FailureInfo {
	if in == nil {
		return nil
	}
	out := new(FailureInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceService) DeepCopyInto(out *InferenceService) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceService.
func (in *InferenceService) DeepCopy() *InferenceService {
	if in == nil {
		return nil
	}
	out := new(InferenceService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InferenceService) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceServiceList) DeepCopyInto(out *InferenceServiceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]InferenceService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceServiceList.
func (in *InferenceServiceList) DeepCopy() *InferenceServiceList {
	if in == nil {
		return nil
	}
	out := new(InferenceServiceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InferenceServiceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceServiceSpec) DeepCopyInto(out *InferenceServiceSpec) {
	*out = *in
	in.Predictor.DeepCopyInto(&out.Predictor)
	if in.Explainer != nil {
		in, out := &in.Explainer, &out.Explainer
		*out = new(ExplainerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Transformer != nil {
		in, out := &in.Transformer, &out.Transformer
		*out = new(TransformerSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceServiceSpec.
func (in *InferenceServiceSpec) DeepCopy() *InferenceServiceSpec {
	if in == nil {
		return nil
	}
	out := new(InferenceServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceServiceStatus) DeepCopyInto(out *InferenceServiceStatus) {
	*out = *in
	in.Status.DeepCopyInto(&out.Status)
	if in.Address != nil {
		in, out := &in.Address, &out.Address
		*out = new(v1.Addressable)
		(*in).DeepCopyInto(*out)
	}
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
	if in.Components != nil {
		in, out := &in.Components, &out.Components
		*out = make(map[ComponentType]ComponentStatusSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	in.ModelStatus.DeepCopyInto(&out.ModelStatus)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceServiceStatus.
func (in *InferenceServiceStatus) DeepCopy() *InferenceServiceStatus {
	if in == nil {
		return nil
	}
	out := new(InferenceServiceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LightGBMSpec) DeepCopyInto(out *LightGBMSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LightGBMSpec.
func (in *LightGBMSpec) DeepCopy() *LightGBMSpec {
	if in == nil {
		return nil
	}
	out := new(LightGBMSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoggerSpec) DeepCopyInto(out *LoggerSpec) {
	*out = *in
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoggerSpec.
func (in *LoggerSpec) DeepCopy() *LoggerSpec {
	if in == nil {
		return nil
	}
	out := new(LoggerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelCopies) DeepCopyInto(out *ModelCopies) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelCopies.
func (in *ModelCopies) DeepCopy() *ModelCopies {
	if in == nil {
		return nil
	}
	out := new(ModelCopies)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelFormat) DeepCopyInto(out *ModelFormat) {
	*out = *in
	if in.Version != nil {
		in, out := &in.Version, &out.Version
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelFormat.
func (in *ModelFormat) DeepCopy() *ModelFormat {
	if in == nil {
		return nil
	}
	out := new(ModelFormat)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelRevisionStates) DeepCopyInto(out *ModelRevisionStates) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelRevisionStates.
func (in *ModelRevisionStates) DeepCopy() *ModelRevisionStates {
	if in == nil {
		return nil
	}
	out := new(ModelRevisionStates)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelSpec) DeepCopyInto(out *ModelSpec) {
	*out = *in
	in.ModelFormat.DeepCopyInto(&out.ModelFormat)
	if in.Runtime != nil {
		in, out := &in.Runtime, &out.Runtime
		*out = new(string)
		**out = **in
	}
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelSpec.
func (in *ModelSpec) DeepCopy() *ModelSpec {
	if in == nil {
		return nil
	}
	out := new(ModelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelStatus) DeepCopyInto(out *ModelStatus) {
	*out = *in
	if in.ModelRevisionStates != nil {
		in, out := &in.ModelRevisionStates, &out.ModelRevisionStates
		*out = new(ModelRevisionStates)
		**out = **in
	}
	if in.LastFailureInfo != nil {
		in, out := &in.LastFailureInfo, &out.LastFailureInfo
		*out = new(FailureInfo)
		(*in).DeepCopyInto(*out)
	}
	if in.ModelCopies != nil {
		in, out := &in.ModelCopies, &out.ModelCopies
		*out = new(ModelCopies)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelStatus.
func (in *ModelStatus) DeepCopy() *ModelStatus {
	if in == nil {
		return nil
	}
	out := new(ModelStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ONNXRuntimeSpec) DeepCopyInto(out *ONNXRuntimeSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ONNXRuntimeSpec.
func (in *ONNXRuntimeSpec) DeepCopy() *ONNXRuntimeSpec {
	if in == nil {
		return nil
	}
	out := new(ONNXRuntimeSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PMMLSpec) DeepCopyInto(out *PMMLSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PMMLSpec.
func (in *PMMLSpec) DeepCopy() *PMMLSpec {
	if in == nil {
		return nil
	}
	out := new(PMMLSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaddleServerSpec) DeepCopyInto(out *PaddleServerSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaddleServerSpec.
func (in *PaddleServerSpec) DeepCopy() *PaddleServerSpec {
	if in == nil {
		return nil
	}
	out := new(PaddleServerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodSpec) DeepCopyInto(out *PodSpec) {
	*out = *in
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]corev1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.InitContainers != nil {
		in, out := &in.InitContainers, &out.InitContainers
		*out = make([]corev1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]corev1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.EphemeralContainers != nil {
		in, out := &in.EphemeralContainers, &out.EphemeralContainers
		*out = make([]corev1.EphemeralContainer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TerminationGracePeriodSeconds != nil {
		in, out := &in.TerminationGracePeriodSeconds, &out.TerminationGracePeriodSeconds
		*out = new(int64)
		**out = **in
	}
	if in.ActiveDeadlineSeconds != nil {
		in, out := &in.ActiveDeadlineSeconds, &out.ActiveDeadlineSeconds
		*out = new(int64)
		**out = **in
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.AutomountServiceAccountToken != nil {
		in, out := &in.AutomountServiceAccountToken, &out.AutomountServiceAccountToken
		*out = new(bool)
		**out = **in
	}
	if in.ShareProcessNamespace != nil {
		in, out := &in.ShareProcessNamespace, &out.ShareProcessNamespace
		*out = new(bool)
		**out = **in
	}
	if in.SecurityContext != nil {
		in, out := &in.SecurityContext, &out.SecurityContext
		*out = new(corev1.PodSecurityContext)
		(*in).DeepCopyInto(*out)
	}
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]corev1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.HostAliases != nil {
		in, out := &in.HostAliases, &out.HostAliases
		*out = make([]corev1.HostAlias, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Priority != nil {
		in, out := &in.Priority, &out.Priority
		*out = new(int32)
		**out = **in
	}
	if in.DNSConfig != nil {
		in, out := &in.DNSConfig, &out.DNSConfig
		*out = new(corev1.PodDNSConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.ReadinessGates != nil {
		in, out := &in.ReadinessGates, &out.ReadinessGates
		*out = make([]corev1.PodReadinessGate, len(*in))
		copy(*out, *in)
	}
	if in.RuntimeClassName != nil {
		in, out := &in.RuntimeClassName, &out.RuntimeClassName
		*out = new(string)
		**out = **in
	}
	if in.EnableServiceLinks != nil {
		in, out := &in.EnableServiceLinks, &out.EnableServiceLinks
		*out = new(bool)
		**out = **in
	}
	if in.PreemptionPolicy != nil {
		in, out := &in.PreemptionPolicy, &out.PreemptionPolicy
		*out = new(corev1.PreemptionPolicy)
		**out = **in
	}
	if in.Overhead != nil {
		in, out := &in.Overhead, &out.Overhead
		*out = make(corev1.ResourceList, len(*in))
		for key, val := range *in {
			(*out)[key] = val.DeepCopy()
		}
	}
	if in.TopologySpreadConstraints != nil {
		in, out := &in.TopologySpreadConstraints, &out.TopologySpreadConstraints
		*out = make([]corev1.TopologySpreadConstraint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SetHostnameAsFQDN != nil {
		in, out := &in.SetHostnameAsFQDN, &out.SetHostnameAsFQDN
		*out = new(bool)
		**out = **in
	}
	if in.OS != nil {
		in, out := &in.OS, &out.OS
		*out = new(corev1.PodOS)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodSpec.
func (in *PodSpec) DeepCopy() *PodSpec {
	if in == nil {
		return nil
	}
	out := new(PodSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PredictorExtensionSpec) DeepCopyInto(out *PredictorExtensionSpec) {
	*out = *in
	if in.StorageURI != nil {
		in, out := &in.StorageURI, &out.StorageURI
		*out = new(string)
		**out = **in
	}
	if in.RuntimeVersion != nil {
		in, out := &in.RuntimeVersion, &out.RuntimeVersion
		*out = new(string)
		**out = **in
	}
	if in.ProtocolVersion != nil {
		in, out := &in.ProtocolVersion, &out.ProtocolVersion
		*out = new(constants.InferenceServiceProtocol)
		**out = **in
	}
	in.Container.DeepCopyInto(&out.Container)
	if in.Storage != nil {
		in, out := &in.Storage, &out.Storage
		*out = new(StorageSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PredictorExtensionSpec.
func (in *PredictorExtensionSpec) DeepCopy() *PredictorExtensionSpec {
	if in == nil {
		return nil
	}
	out := new(PredictorExtensionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PredictorSpec) DeepCopyInto(out *PredictorSpec) {
	*out = *in
	if in.SKLearn != nil {
		in, out := &in.SKLearn, &out.SKLearn
		*out = new(SKLearnSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.XGBoost != nil {
		in, out := &in.XGBoost, &out.XGBoost
		*out = new(XGBoostSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Tensorflow != nil {
		in, out := &in.Tensorflow, &out.Tensorflow
		*out = new(TFServingSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.PyTorch != nil {
		in, out := &in.PyTorch, &out.PyTorch
		*out = new(TorchServeSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Triton != nil {
		in, out := &in.Triton, &out.Triton
		*out = new(TritonSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.ONNX != nil {
		in, out := &in.ONNX, &out.ONNX
		*out = new(ONNXRuntimeSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.PMML != nil {
		in, out := &in.PMML, &out.PMML
		*out = new(PMMLSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.LightGBM != nil {
		in, out := &in.LightGBM, &out.LightGBM
		*out = new(LightGBMSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Paddle != nil {
		in, out := &in.Paddle, &out.Paddle
		*out = new(PaddleServerSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Model != nil {
		in, out := &in.Model, &out.Model
		*out = new(ModelSpec)
		(*in).DeepCopyInto(*out)
	}
	in.PodSpec.DeepCopyInto(&out.PodSpec)
	in.ComponentExtensionSpec.DeepCopyInto(&out.ComponentExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PredictorSpec.
func (in *PredictorSpec) DeepCopy() *PredictorSpec {
	if in == nil {
		return nil
	}
	out := new(PredictorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SKLearnSpec) DeepCopyInto(out *SKLearnSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SKLearnSpec.
func (in *SKLearnSpec) DeepCopy() *SKLearnSpec {
	if in == nil {
		return nil
	}
	out := new(SKLearnSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageSpec) DeepCopyInto(out *StorageSpec) {
	*out = *in
	if in.Path != nil {
		in, out := &in.Path, &out.Path
		*out = new(string)
		**out = **in
	}
	if in.SchemaPath != nil {
		in, out := &in.SchemaPath, &out.SchemaPath
		*out = new(string)
		**out = **in
	}
	if in.Parameters != nil {
		in, out := &in.Parameters, &out.Parameters
		*out = new(map[string]string)
		if **in != nil {
			in, out := *in, *out
			*out = make(map[string]string, len(*in))
			for key, val := range *in {
				(*out)[key] = val
			}
		}
	}
	if in.StorageKey != nil {
		in, out := &in.StorageKey, &out.StorageKey
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageSpec.
func (in *StorageSpec) DeepCopy() *StorageSpec {
	if in == nil {
		return nil
	}
	out := new(StorageSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TFServingSpec) DeepCopyInto(out *TFServingSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TFServingSpec.
func (in *TFServingSpec) DeepCopy() *TFServingSpec {
	if in == nil {
		return nil
	}
	out := new(TFServingSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TorchServeSpec) DeepCopyInto(out *TorchServeSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TorchServeSpec.
func (in *TorchServeSpec) DeepCopy() *TorchServeSpec {
	if in == nil {
		return nil
	}
	out := new(TorchServeSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TransformerSpec) DeepCopyInto(out *TransformerSpec) {
	*out = *in
	in.PodSpec.DeepCopyInto(&out.PodSpec)
	in.ComponentExtensionSpec.DeepCopyInto(&out.ComponentExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TransformerSpec.
func (in *TransformerSpec) DeepCopy() *TransformerSpec {
	if in == nil {
		return nil
	}
	out := new(TransformerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TritonSpec) DeepCopyInto(out *TritonSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TritonSpec.
func (in *TritonSpec) DeepCopy() *TritonSpec {
	if in == nil {
		return nil
	}
	out := new(TritonSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *XGBoostSpec) DeepCopyInto(out *XGBoostSpec) {
	*out = *in
	in.PredictorExtensionSpec.DeepCopyInto(&out.PredictorExtensionSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new XGBoostSpec.
func (in *XGBoostSpec) DeepCopy() *XGBoostSpec {
	if in == nil {
		return nil
	}
	out := new(XGBoostSpec)
	in.DeepCopyInto(out)
	return out
}
