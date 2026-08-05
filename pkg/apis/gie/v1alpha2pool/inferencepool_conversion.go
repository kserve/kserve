/*
Copyright 2025 The Kubernetes Authors.
Copyright 2026 The KServe Authors.

Vendored from sigs.k8s.io/gateway-api-inference-extension@v1.4.0/apix/v1alpha2/
with modifications for KServe's local v1alpha2 InferencePool shim.

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

package v1alpha2pool

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
)

// ConvertTo converts this InferencePool (v1alpha2) to the v1 version.
func (src *InferencePool) ConvertTo(dst *v1.InferencePool) error {
	if dst == nil {
		return errors.New("dst cannot be nil")
	}
	endpointPickRef, err := convertExtensionRefToV1(&src.Spec.ExtensionRef)
	if err != nil {
		return err
	}
	v1Status, err := convertStatusToV1(&src.Status)
	if err != nil {
		return err
	}

	meta := metav1.TypeMeta{
		Kind:       src.Kind,
		APIVersion: v1.GroupVersion.String(),
	}
	dst.TypeMeta = meta
	src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	dst.Spec.TargetPorts = []v1.Port{{Number: v1.PortNumber(src.Spec.TargetPortNumber)}}
	dst.Spec.EndpointPickerRef = endpointPickRef
	dst.Status = *v1Status

	if src.Spec.Selector != nil {
		dst.Spec.Selector.MatchLabels = make(map[v1.LabelKey]v1.LabelValue, len(src.Spec.Selector))
		for k, val := range src.Spec.Selector {
			dst.Spec.Selector.MatchLabels[v1.LabelKey(k)] = v1.LabelValue(val)
		}
	}
	return nil
}

// ConvertFrom converts from the v1 version to this version (v1alpha2).
func (dst *InferencePool) ConvertFrom(src *v1.InferencePool) error {
	if src == nil {
		return errors.New("src cannot be nil")
	}
	extensionRef, err := convertEndpointPickerRefFromV1(&src.Spec.EndpointPickerRef)
	if err != nil {
		return err
	}
	status, err := convertStatusFromV1(&src.Status)
	if err != nil {
		return err
	}

	meta := metav1.TypeMeta{
		Kind:       src.Kind,
		APIVersion: GroupVersion.String(),
	}
	dst.TypeMeta = meta
	src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	dst.Spec.TargetPortNumber = int32(src.Spec.TargetPorts[0].Number)
	dst.Spec.ExtensionRef = extensionRef
	dst.Status = *status

	if src.Spec.Selector.MatchLabels != nil {
		dst.Spec.Selector = make(map[LabelKey]LabelValue, len(src.Spec.Selector.MatchLabels))
		for k, val := range src.Spec.Selector.MatchLabels {
			dst.Spec.Selector[LabelKey(k)] = LabelValue(val)
		}
	}
	return nil
}

func convertStatusToV1(src *InferencePoolStatus) (*v1.InferencePoolStatus, error) {
	if src == nil {
		return nil, errors.New("src cannot be nil")
	}
	if len(src.Parents) == 0 {
		return &v1.InferencePoolStatus{}, nil
	}
	out := &v1.InferencePoolStatus{
		Parents: make([]v1.ParentStatus, 0, len(src.Parents)),
	}
	for _, p := range src.Parents {
		if isV1Alpha2DefaultParent(p) {
			continue
		}
		ps := v1.ParentStatus{
			ParentRef:  toV1ParentRef(p.GatewayRef),
			Conditions: nil,
		}
		for _, c := range p.Conditions {
			if isV1Alpha2DefaultCondition(c) {
				continue
			}
			cc := c
			if cc.Type == string(v1.InferencePoolConditionAccepted) {
				cc.Reason = mapAcceptedReasonToV1(cc.Reason)
			}
			ps.Conditions = append(ps.Conditions, cc)
		}

		out.Parents = append(out.Parents, ps)
	}

	if len(out.Parents) == 0 {
		return &v1.InferencePoolStatus{}, nil
	}
	return out, nil
}

func convertStatusFromV1(src *v1.InferencePoolStatus) (*InferencePoolStatus, error) {
	if src == nil {
		return nil, errors.New("src cannot be nil")
	}
	if len(src.Parents) == 0 {
		return &InferencePoolStatus{}, nil
	}
	out := &InferencePoolStatus{
		Parents: make([]PoolStatus, 0, len(src.Parents)),
	}
	for _, p := range src.Parents {
		ps := PoolStatus{
			GatewayRef: fromV1ParentRef(p.ParentRef),
		}
		if n := len(p.Conditions); n > 0 {
			ps.Conditions = make([]metav1.Condition, 0, n)
			for _, c := range p.Conditions {
				cc := c
				if cc.Type == string(v1.InferencePoolConditionAccepted) {
					cc.Reason = mapAcceptedReasonFromV1(cc.Reason)
				}
				ps.Conditions = append(ps.Conditions, cc)
			}
		}
		out.Parents = append(out.Parents, ps)
	}
	return out, nil
}

func isV1Alpha2DefaultParent(p PoolStatus) bool {
	if p.GatewayRef.Kind == nil || p.GatewayRef.Name == "" {
		return false
	}
	return *p.GatewayRef.Kind == "Status" && p.GatewayRef.Name == "default"
}

func mapAcceptedReasonToV1(r string) string {
	switch InferencePoolReason(r) {
	case InferencePoolReasonAccepted:
		return string(v1.InferencePoolReasonAccepted)
	case InferencePoolReasonNotSupportedByGateway:
		return string(v1.InferencePoolReasonNotSupportedByParent)
	default:
		return r
	}
}

func mapAcceptedReasonFromV1(r string) string {
	switch v1.InferencePoolReason(r) {
	case v1.InferencePoolReasonAccepted:
		return string(InferencePoolReasonAccepted)
	case v1.InferencePoolReasonNotSupportedByParent:
		return string(InferencePoolReasonNotSupportedByGateway)
	default:
		return r
	}
}

func isV1Alpha2DefaultCondition(c metav1.Condition) bool {
	return InferencePoolConditionType(c.Type) == InferencePoolConditionAccepted &&
		c.Status == metav1.ConditionUnknown &&
		InferencePoolReason(c.Reason) == InferencePoolReasonPending
}

func toV1ParentRef(in ParentGatewayReference) v1.ParentReference {
	out := v1.ParentReference{
		Name: v1.ObjectName(in.Name),
	}
	if in.Group != nil {
		g := v1.Group(*in.Group)
		out.Group = &g
	}
	k := v1.Kind("Gateway")
	if in.Kind != nil {
		k = v1.Kind(*in.Kind)
	}
	out.Kind = k
	if in.Namespace != nil {
		ns := v1.Namespace(*in.Namespace)
		out.Namespace = ns
	}
	return out
}

func fromV1ParentRef(in v1.ParentReference) ParentGatewayReference {
	out := ParentGatewayReference{
		Name: ObjectName(in.Name),
	}
	if in.Group != nil {
		g := Group(*in.Group)
		out.Group = &g
	}
	kk := Kind(in.Kind)
	out.Kind = &kk
	if in.Namespace != "" {
		ns := Namespace(in.Namespace)
		out.Namespace = &ns
	}
	return out
}

func convertExtensionRefToV1(src *Extension) (v1.EndpointPickerRef, error) {
	endpointPickerRef := v1.EndpointPickerRef{}
	if src == nil {
		return endpointPickerRef, errors.New("src cannot be nil")
	}
	if src.Group != nil {
		endpointPickerRef.Group = ptr.To(v1.Group(*src.Group))
	}
	kind := v1.Kind("Service")
	if src.Kind != nil {
		kind = v1.Kind(*src.Kind)
	}
	endpointPickerRef.Kind = kind
	endpointPickerRef.Name = v1.ObjectName(src.Name)
	if src.PortNumber != nil {
		endpointPickerRef.Port = ptr.To(v1.Port{Number: v1.PortNumber(*src.PortNumber)})
	}
	failureMode := v1.EndpointPickerFailClose
	if src.FailureMode != nil {
		failureMode = v1.EndpointPickerFailureMode(*src.FailureMode)
	}
	endpointPickerRef.FailureMode = failureMode

	// The v1 InferencePool CRD requires port when kind is "Service" or unspecified. Old
	// v1alpha2 pools may omit PortNumber, which would make the converted v1 object fail
	// the CEL rule. Default to the standard EPP gRPC port (9002).
	if (endpointPickerRef.Port == nil || endpointPickerRef.Port.Number == 0) && (endpointPickerRef.Kind == "Service" || endpointPickerRef.Kind == "") {
		endpointPickerRef.Port = ptr.To(v1.Port{Number: 9002})
	}

	return endpointPickerRef, nil
}

func convertEndpointPickerRefFromV1(src *v1.EndpointPickerRef) (Extension, error) {
	extension := Extension{}
	if src == nil {
		return extension, errors.New("src cannot be nil")
	}
	if src.Group != nil {
		extension.Group = ptr.To(Group(*src.Group))
	}
	extension.Kind = ptr.To(Kind(src.Kind))
	extension.Name = ObjectName(src.Name)
	if src.Port != nil {
		extension.PortNumber = ptr.To(PortNumber(src.Port.Number))
	}
	extension.FailureMode = ptr.To(ExtensionFailureMode(src.FailureMode))
	return extension, nil
}
