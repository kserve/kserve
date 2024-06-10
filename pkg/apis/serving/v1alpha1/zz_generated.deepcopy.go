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

package v1alpha1

import (
	"github.com/kserve/kserve/pkg/constants"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BuiltInAdapter) DeepCopyInto(out *BuiltInAdapter) {
	*out = *in
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BuiltInAdapter.
func (in *BuiltInAdapter) DeepCopy() *BuiltInAdapter {
	if in == nil {
		return nil
	}
	out := new(BuiltInAdapter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterCachedModel) DeepCopyInto(out *ClusterCachedModel) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Disabled != nil {
		in, out := &in.Disabled, &out.Disabled
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterCachedModel.
func (in *ClusterCachedModel) DeepCopy() *ClusterCachedModel {
	if in == nil {
		return nil
	}
	out := new(ClusterCachedModel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterCachedModel) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterCachedModelList) DeepCopyInto(out *ClusterCachedModelList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterCachedModel, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterCachedModelList.
func (in *ClusterCachedModelList) DeepCopy() *ClusterCachedModelList {
	if in == nil {
		return nil
	}
	out := new(ClusterCachedModelList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterCachedModelList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterCachedModelSpec) DeepCopyInto(out *ClusterCachedModelSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterCachedModelSpec.
func (in *ClusterCachedModelSpec) DeepCopy() *ClusterCachedModelSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterCachedModelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterServingRuntime) DeepCopyInto(out *ClusterServingRuntime) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterServingRuntime.
func (in *ClusterServingRuntime) DeepCopy() *ClusterServingRuntime {
	if in == nil {
		return nil
	}
	out := new(ClusterServingRuntime)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterServingRuntime) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterServingRuntimeList) DeepCopyInto(out *ClusterServingRuntimeList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterServingRuntime, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterServingRuntimeList.
func (in *ClusterServingRuntimeList) DeepCopy() *ClusterServingRuntimeList {
	if in == nil {
		return nil
	}
	out := new(ClusterServingRuntimeList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterServingRuntimeList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterStorageContainer) DeepCopyInto(out *ClusterStorageContainer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Disabled != nil {
		in, out := &in.Disabled, &out.Disabled
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterStorageContainer.
func (in *ClusterStorageContainer) DeepCopy() *ClusterStorageContainer {
	if in == nil {
		return nil
	}
	out := new(ClusterStorageContainer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterStorageContainer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterStorageContainerList) DeepCopyInto(out *ClusterStorageContainerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterStorageContainer, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterStorageContainerList.
func (in *ClusterStorageContainerList) DeepCopy() *ClusterStorageContainerList {
	if in == nil {
		return nil
	}
	out := new(ClusterStorageContainerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterStorageContainerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceGraph) DeepCopyInto(out *InferenceGraph) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceGraph.
func (in *InferenceGraph) DeepCopy() *InferenceGraph {
	if in == nil {
		return nil
	}
	out := new(InferenceGraph)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InferenceGraph) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceGraphList) DeepCopyInto(out *InferenceGraphList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]InferenceGraph, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceGraphList.
func (in *InferenceGraphList) DeepCopy() *InferenceGraphList {
	if in == nil {
		return nil
	}
	out := new(InferenceGraphList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InferenceGraphList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceGraphSpec) DeepCopyInto(out *InferenceGraphSpec) {
	*out = *in
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make(map[string]InferenceRouter, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.TimeoutSeconds != nil {
		in, out := &in.TimeoutSeconds, &out.TimeoutSeconds
		*out = new(int64)
		**out = **in
	}
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
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceGraphSpec.
func (in *InferenceGraphSpec) DeepCopy() *InferenceGraphSpec {
	if in == nil {
		return nil
	}
	out := new(InferenceGraphSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceGraphStatus) DeepCopyInto(out *InferenceGraphStatus) {
	*out = *in
	in.Status.DeepCopyInto(&out.Status)
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceGraphStatus.
func (in *InferenceGraphStatus) DeepCopy() *InferenceGraphStatus {
	if in == nil {
		return nil
	}
	out := new(InferenceGraphStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceRouter) DeepCopyInto(out *InferenceRouter) {
	*out = *in
	if in.Steps != nil {
		in, out := &in.Steps, &out.Steps
		*out = make([]InferenceStep, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceRouter.
func (in *InferenceRouter) DeepCopy() *InferenceRouter {
	if in == nil {
		return nil
	}
	out := new(InferenceRouter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceStep) DeepCopyInto(out *InferenceStep) {
	*out = *in
	out.InferenceTarget = in.InferenceTarget
	if in.Weight != nil {
		in, out := &in.Weight, &out.Weight
		*out = new(int64)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceStep.
func (in *InferenceStep) DeepCopy() *InferenceStep {
	if in == nil {
		return nil
	}
	out := new(InferenceStep)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InferenceTarget) DeepCopyInto(out *InferenceTarget) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InferenceTarget.
func (in *InferenceTarget) DeepCopy() *InferenceTarget {
	if in == nil {
		return nil
	}
	out := new(InferenceTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelSpec) DeepCopyInto(out *ModelSpec) {
	*out = *in
	out.Memory = in.Memory.DeepCopy()
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
func (in *ServingRuntime) DeepCopyInto(out *ServingRuntime) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServingRuntime.
func (in *ServingRuntime) DeepCopy() *ServingRuntime {
	if in == nil {
		return nil
	}
	out := new(ServingRuntime)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServingRuntime) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServingRuntimeList) DeepCopyInto(out *ServingRuntimeList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ServingRuntime, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServingRuntimeList.
func (in *ServingRuntimeList) DeepCopy() *ServingRuntimeList {
	if in == nil {
		return nil
	}
	out := new(ServingRuntimeList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ServingRuntimeList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServingRuntimePodSpec) DeepCopyInto(out *ServingRuntimePodSpec) {
	*out = *in
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]v1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]v1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServingRuntimePodSpec.
func (in *ServingRuntimePodSpec) DeepCopy() *ServingRuntimePodSpec {
	if in == nil {
		return nil
	}
	out := new(ServingRuntimePodSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServingRuntimeSpec) DeepCopyInto(out *ServingRuntimeSpec) {
	*out = *in
	if in.SupportedModelFormats != nil {
		in, out := &in.SupportedModelFormats, &out.SupportedModelFormats
		*out = make([]SupportedModelFormat, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.MultiModel != nil {
		in, out := &in.MultiModel, &out.MultiModel
		*out = new(bool)
		**out = **in
	}
	if in.Disabled != nil {
		in, out := &in.Disabled, &out.Disabled
		*out = new(bool)
		**out = **in
	}
	if in.ProtocolVersions != nil {
		in, out := &in.ProtocolVersions, &out.ProtocolVersions
		*out = make([]constants.InferenceServiceProtocol, len(*in))
		copy(*out, *in)
	}
	in.ServingRuntimePodSpec.DeepCopyInto(&out.ServingRuntimePodSpec)
	if in.GrpcMultiModelManagementEndpoint != nil {
		in, out := &in.GrpcMultiModelManagementEndpoint, &out.GrpcMultiModelManagementEndpoint
		*out = new(string)
		**out = **in
	}
	if in.GrpcDataEndpoint != nil {
		in, out := &in.GrpcDataEndpoint, &out.GrpcDataEndpoint
		*out = new(string)
		**out = **in
	}
	if in.HTTPDataEndpoint != nil {
		in, out := &in.HTTPDataEndpoint, &out.HTTPDataEndpoint
		*out = new(string)
		**out = **in
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(uint16)
		**out = **in
	}
	if in.StorageHelper != nil {
		in, out := &in.StorageHelper, &out.StorageHelper
		*out = new(StorageHelper)
		**out = **in
	}
	if in.BuiltInAdapter != nil {
		in, out := &in.BuiltInAdapter, &out.BuiltInAdapter
		*out = new(BuiltInAdapter)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServingRuntimeSpec.
func (in *ServingRuntimeSpec) DeepCopy() *ServingRuntimeSpec {
	if in == nil {
		return nil
	}
	out := new(ServingRuntimeSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServingRuntimeStatus) DeepCopyInto(out *ServingRuntimeStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServingRuntimeStatus.
func (in *ServingRuntimeStatus) DeepCopy() *ServingRuntimeStatus {
	if in == nil {
		return nil
	}
	out := new(ServingRuntimeStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageContainerSpec) DeepCopyInto(out *StorageContainerSpec) {
	*out = *in
	in.Container.DeepCopyInto(&out.Container)
	if in.SupportedUriFormats != nil {
		in, out := &in.SupportedUriFormats, &out.SupportedUriFormats
		*out = make([]SupportedUriFormat, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageContainerSpec.
func (in *StorageContainerSpec) DeepCopy() *StorageContainerSpec {
	if in == nil {
		return nil
	}
	out := new(StorageContainerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageHelper) DeepCopyInto(out *StorageHelper) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageHelper.
func (in *StorageHelper) DeepCopy() *StorageHelper {
	if in == nil {
		return nil
	}
	out := new(StorageHelper)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SupportedModelFormat) DeepCopyInto(out *SupportedModelFormat) {
	*out = *in
	if in.Version != nil {
		in, out := &in.Version, &out.Version
		*out = new(string)
		**out = **in
	}
	if in.AutoSelect != nil {
		in, out := &in.AutoSelect, &out.AutoSelect
		*out = new(bool)
		**out = **in
	}
	if in.Priority != nil {
		in, out := &in.Priority, &out.Priority
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SupportedModelFormat.
func (in *SupportedModelFormat) DeepCopy() *SupportedModelFormat {
	if in == nil {
		return nil
	}
	out := new(SupportedModelFormat)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SupportedRuntime) DeepCopyInto(out *SupportedRuntime) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SupportedRuntime.
func (in *SupportedRuntime) DeepCopy() *SupportedRuntime {
	if in == nil {
		return nil
	}
	out := new(SupportedRuntime)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SupportedUriFormat) DeepCopyInto(out *SupportedUriFormat) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SupportedUriFormat.
func (in *SupportedUriFormat) DeepCopy() *SupportedUriFormat {
	if in == nil {
		return nil
	}
	out := new(SupportedUriFormat)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrainedModel) DeepCopyInto(out *TrainedModel) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrainedModel.
func (in *TrainedModel) DeepCopy() *TrainedModel {
	if in == nil {
		return nil
	}
	out := new(TrainedModel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TrainedModel) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrainedModelList) DeepCopyInto(out *TrainedModelList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TrainedModel, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrainedModelList.
func (in *TrainedModelList) DeepCopy() *TrainedModelList {
	if in == nil {
		return nil
	}
	out := new(TrainedModelList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TrainedModelList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrainedModelSpec) DeepCopyInto(out *TrainedModelSpec) {
	*out = *in
	in.Model.DeepCopyInto(&out.Model)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrainedModelSpec.
func (in *TrainedModelSpec) DeepCopy() *TrainedModelSpec {
	if in == nil {
		return nil
	}
	out := new(TrainedModelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrainedModelStatus) DeepCopyInto(out *TrainedModelStatus) {
	*out = *in
	in.Status.DeepCopyInto(&out.Status)
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(apis.URL)
		(*in).DeepCopyInto(*out)
	}
	if in.Address != nil {
		in, out := &in.Address, &out.Address
		*out = new(duckv1.Addressable)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrainedModelStatus.
func (in *TrainedModelStatus) DeepCopy() *TrainedModelStatus {
	if in == nil {
		return nil
	}
	out := new(TrainedModelStatus)
	in.DeepCopyInto(out)
	return out
}
