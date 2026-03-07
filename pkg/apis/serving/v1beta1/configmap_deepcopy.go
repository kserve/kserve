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

package v1beta1

// DeepCopyInto copies all fields of IngressConfig into out. in must be non-nil.
func (in *IngressConfig) DeepCopyInto(out *IngressConfig) {
	*out = *in
	if in.IngressClassName != nil {
		in, out := &in.IngressClassName, &out.IngressClassName
		*out = new(string)
		**out = **in
	}
	if in.AdditionalIngressDomains != nil {
		in, out := &in.AdditionalIngressDomains, &out.AdditionalIngressDomains
		*out = new([]string)
		if **in != nil {
			**out = make([]string, len(**in))
			copy(**out, **in)
		}
	}
}

// DeepCopy creates a new IngressConfig by deep copying the receiver.
func (in *IngressConfig) DeepCopy() *IngressConfig {
	if in == nil {
		return nil
	}
	out := new(IngressConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of DeployConfig into out. in must be non-nil.
func (in *DeployConfig) DeepCopyInto(out *DeployConfig) {
	*out = *in
	if in.DeploymentRolloutStrategy != nil {
		in, out := &in.DeploymentRolloutStrategy, &out.DeploymentRolloutStrategy
		*out = new(DeploymentRolloutStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy creates a new DeployConfig by deep copying the receiver.
func (in *DeployConfig) DeepCopy() *DeployConfig {
	if in == nil {
		return nil
	}
	out := new(DeployConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of InferenceServicesConfig into out. in must be non-nil.
func (in *InferenceServicesConfig) DeepCopyInto(out *InferenceServicesConfig) {
	*out = *in
	if in.ServiceAnnotationDisallowedList != nil {
		in, out := &in.ServiceAnnotationDisallowedList, &out.ServiceAnnotationDisallowedList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ServiceLabelDisallowedList != nil {
		in, out := &in.ServiceLabelDisallowedList, &out.ServiceLabelDisallowedList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy creates a new InferenceServicesConfig by deep copying the receiver.
func (in *InferenceServicesConfig) DeepCopy() *InferenceServicesConfig {
	if in == nil {
		return nil
	}
	out := new(InferenceServicesConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of MultiNodeConfig into out. in must be non-nil.
func (in *MultiNodeConfig) DeepCopyInto(out *MultiNodeConfig) {
	*out = *in
	if in.CustomGPUResourceTypeList != nil {
		in, out := &in.CustomGPUResourceTypeList, &out.CustomGPUResourceTypeList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy creates a new MultiNodeConfig by deep copying the receiver.
func (in *MultiNodeConfig) DeepCopy() *MultiNodeConfig {
	if in == nil {
		return nil
	}
	out := new(MultiNodeConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of LocalModelConfig into out. in must be non-nil.
func (in *LocalModelConfig) DeepCopyInto(out *LocalModelConfig) {
	*out = *in
	if in.FSGroup != nil {
		in, out := &in.FSGroup, &out.FSGroup
		*out = new(int64)
		**out = **in
	}
	if in.JobTTLSecondsAfterFinished != nil {
		in, out := &in.JobTTLSecondsAfterFinished, &out.JobTTLSecondsAfterFinished
		*out = new(int32)
		**out = **in
	}
	if in.ReconcilationFrequencyInSecs != nil {
		in, out := &in.ReconcilationFrequencyInSecs, &out.ReconcilationFrequencyInSecs
		*out = new(int64)
		**out = **in
	}
}

// DeepCopy creates a new LocalModelConfig by deep copying the receiver.
func (in *LocalModelConfig) DeepCopy() *LocalModelConfig {
	if in == nil {
		return nil
	}
	out := new(LocalModelConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of SecurityConfig into out. in must be non-nil.
func (in *SecurityConfig) DeepCopyInto(out *SecurityConfig) {
	*out = *in
}

// DeepCopy creates a new SecurityConfig by deep copying the receiver.
func (in *SecurityConfig) DeepCopy() *SecurityConfig {
	if in == nil {
		return nil
	}
	out := new(SecurityConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields of ServiceConfig into out. in must be non-nil.
func (in *ServiceConfig) DeepCopyInto(out *ServiceConfig) {
	*out = *in
}

// DeepCopy creates a new ServiceConfig by deep copying the receiver.
func (in *ServiceConfig) DeepCopy() *ServiceConfig {
	if in == nil {
		return nil
	}
	out := new(ServiceConfig)
	in.DeepCopyInto(out)
	return out
}
