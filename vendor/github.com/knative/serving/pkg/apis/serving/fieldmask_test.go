/*
Copyright 2019 The Knative Authors

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

package serving

import (
	"testing"

	"github.com/knative/pkg/kmp"
	"github.com/knative/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestVolumeMask(t *testing.T) {
	want := &corev1.Volume{
		Name:         "foo",
		VolumeSource: corev1.VolumeSource{},
	}
	in := want

	got := VolumeMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("VolumeMask (-want, +got): %s", diff)
	}

	if got = VolumeMask(nil); got != nil {
		t.Errorf("VolumeMask(nil) = %v, want: nil", got)
	}
}

func TestVolumeSourceMask(t *testing.T) {
	want := &corev1.VolumeSource{
		Secret:    &corev1.SecretVolumeSource{},
		ConfigMap: &corev1.ConfigMapVolumeSource{},
	}
	in := &corev1.VolumeSource{
		Secret:    &corev1.SecretVolumeSource{},
		ConfigMap: &corev1.ConfigMapVolumeSource{},
		NFS:       &corev1.NFSVolumeSource{},
	}

	got := VolumeSourceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("VolumeSourceMask (-want, +got): %s", diff)
	}

	if got = VolumeSourceMask(nil); got != nil {
		t.Errorf("VolumeSourceMask(nil) = %v, want: nil", got)
	}
}

func TestContainerMask(t *testing.T) {
	want := &corev1.Container{
		Args:                     []string{"hello"},
		Command:                  []string{"world"},
		Env:                      []corev1.EnvVar{{}},
		EnvFrom:                  []corev1.EnvFromSource{{}},
		Image:                    "python",
		LivenessProbe:            &corev1.Probe{},
		Ports:                    []corev1.ContainerPort{{}},
		ReadinessProbe:           &corev1.Probe{},
		Resources:                corev1.ResourceRequirements{},
		SecurityContext:          &corev1.SecurityContext{},
		TerminationMessagePath:   "/",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		VolumeMounts:             []corev1.VolumeMount{{}},
	}
	in := &corev1.Container{
		Args:                     []string{"hello"},
		Command:                  []string{"world"},
		Env:                      []corev1.EnvVar{{}},
		EnvFrom:                  []corev1.EnvFromSource{{}},
		Image:                    "python",
		LivenessProbe:            &corev1.Probe{},
		Ports:                    []corev1.ContainerPort{{}},
		ReadinessProbe:           &corev1.Probe{},
		Resources:                corev1.ResourceRequirements{},
		SecurityContext:          &corev1.SecurityContext{},
		TerminationMessagePath:   "/",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		VolumeMounts:             []corev1.VolumeMount{{}},
		Name:                     "foo",
		Stdin:                    true,
		StdinOnce:                true,
		TTY:                      true,
	}

	got := ContainerMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ContainerMask (-want, +got): %s", diff)
	}

	if got = ContainerMask(nil); got != nil {
		t.Errorf("ContainerMask(nil) = %v, want: nil", got)
	}
}

func TestVolumeMountMask(t *testing.T) {
	mode := corev1.MountPropagationBidirectional

	want := &corev1.VolumeMount{
		Name:      "foo",
		ReadOnly:  true,
		MountPath: "/foo/bar",
	}
	in := &corev1.VolumeMount{
		Name:             "foo",
		ReadOnly:         true,
		MountPath:        "/foo/bar",
		SubPath:          "baz/",
		MountPropagation: &mode,
	}

	got := VolumeMountMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("VolumeMountMask (-want, +got): %s", diff)
	}

	if got = VolumeMountMask(nil); got != nil {
		t.Errorf("VolumeMountMask(nil) = %v, want: nil", got)
	}
}

func TestProbeMask(t *testing.T) {
	want := &corev1.Probe{
		Handler:             corev1.Handler{},
		InitialDelaySeconds: 42,
		TimeoutSeconds:      42,
		PeriodSeconds:       42,
		SuccessThreshold:    42,
		FailureThreshold:    42,
	}
	in := want

	got := ProbeMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ProbeMask (-want, +got): %s", diff)
	}

	if got = ProbeMask(nil); got != nil {
		t.Errorf("ProbeMask(nil) = %v, want: nil", got)
	}
}

func TestHandlerMask(t *testing.T) {
	want := &corev1.Handler{
		Exec:      &corev1.ExecAction{},
		HTTPGet:   &corev1.HTTPGetAction{},
		TCPSocket: &corev1.TCPSocketAction{},
	}
	in := want

	got := HandlerMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("HandlerMask (-want, +got): %s", diff)
	}

	if got = HandlerMask(nil); got != nil {
		t.Errorf("HandlerMask(nil) = %v, want: nil", got)
	}
}

func TestExecActionMask(t *testing.T) {
	want := &corev1.ExecAction{
		Command: []string{"foo", "bar"},
	}
	in := want

	got := ExecActionMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ExecActionMask (-want, +got): %s", diff)
	}

	if got = ExecActionMask(nil); got != nil {
		t.Errorf("ExecActionMask(nil) = %v, want: nil", got)
	}
}

func TestHTTPGetActionMask(t *testing.T) {
	want := &corev1.HTTPGetAction{
		Host:        "foo",
		Path:        "/bar",
		Scheme:      corev1.URISchemeHTTP,
		HTTPHeaders: []corev1.HTTPHeader{{}},
	}
	in := &corev1.HTTPGetAction{
		Host:        "foo",
		Path:        "/bar",
		Scheme:      corev1.URISchemeHTTP,
		HTTPHeaders: []corev1.HTTPHeader{{}},
		Port:        intstr.FromInt(8080),
	}

	got := HTTPGetActionMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("HTTPGetActionMask (-want, +got): %s", diff)
	}

	if got = HTTPGetActionMask(nil); got != nil {
		t.Errorf("HTTPGetActionMask(nil) = %v, want: nil", got)
	}
}

func TestTCPSocketActionMask(t *testing.T) {
	want := &corev1.TCPSocketAction{
		Host: "foo",
	}
	in := &corev1.TCPSocketAction{
		Host: "foo",
		Port: intstr.FromInt(8080),
	}

	got := TCPSocketActionMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("TCPSocketActionMask (-want, +got): %s", diff)
	}

	if got = TCPSocketActionMask(nil); got != nil {
		t.Errorf("TCPSocketActionMask(nil) = %v, want: nil", got)
	}
}

func TestContainerPortMask(t *testing.T) {
	want := &corev1.ContainerPort{
		ContainerPort: 42,
		Name:          "foo",
		Protocol:      corev1.ProtocolTCP,
	}
	in := &corev1.ContainerPort{
		ContainerPort: 42,
		Name:          "foo",
		Protocol:      corev1.ProtocolTCP,
		HostIP:        "10.0.0.1",
		HostPort:      43,
	}

	got := ContainerPortMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ContainerPortMask (-want, +got): %s", diff)
	}

	if got = ContainerPortMask(nil); got != nil {
		t.Errorf("ContainerPortMask(nil) = %v, want: nil", got)
	}
}

func TestEnvVarMask(t *testing.T) {
	want := &corev1.EnvVar{
		Name:      "foo",
		Value:     "bar",
		ValueFrom: &corev1.EnvVarSource{},
	}
	in := want

	got := EnvVarMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("EnvVarMask (-want, +got): %s", diff)
	}

	if got = EnvVarMask(nil); got != nil {
		t.Errorf("EnvVarMask(nil) = %v, want: nil", got)
	}
}

func TestEnvVarSourceMask(t *testing.T) {
	want := &corev1.EnvVarSource{
		ConfigMapKeyRef: &corev1.ConfigMapKeySelector{},
		SecretKeyRef:    &corev1.SecretKeySelector{},
	}
	in := &corev1.EnvVarSource{
		ConfigMapKeyRef:  &corev1.ConfigMapKeySelector{},
		SecretKeyRef:     &corev1.SecretKeySelector{},
		FieldRef:         &corev1.ObjectFieldSelector{},
		ResourceFieldRef: &corev1.ResourceFieldSelector{},
	}

	got := EnvVarSourceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("EnvVarSourceMask (-want, +got): %s", diff)
	}

	if got = EnvVarSourceMask(nil); got != nil {
		t.Errorf("EnvVarSourceMask(nil) = %v, want: nil", got)
	}
}

func TestLocalObjectReferenceMask(t *testing.T) {
	want := &corev1.LocalObjectReference{
		Name: "foo",
	}
	in := want

	got := LocalObjectReferenceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("LocalObjectReferenceMask (-want, +got): %s", diff)
	}

	if got = LocalObjectReferenceMask(nil); got != nil {
		t.Errorf("LocalObjectReferenceMask(nil) = %v, want: nil", got)
	}
}

func TestConfigMapKeySelectorMask(t *testing.T) {
	want := &corev1.ConfigMapKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{},
		Key:                  "foo",
		Optional:             ptr.Bool(true),
	}
	in := want

	got := ConfigMapKeySelectorMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ConfigMapKeySelectorMask (-want, +got): %s", diff)
	}

	if got = ConfigMapKeySelectorMask(nil); got != nil {
		t.Errorf("ConfigMapKeySelectorMask(nil) = %v, want: nil", got)
	}
}

func TestSecretKeySelectorMask(t *testing.T) {
	want := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{},
		Key:                  "foo",
		Optional:             ptr.Bool(true),
	}
	in := want

	got := SecretKeySelectorMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("SecretKeySelectorMask (-want, +got): %s", diff)
	}

	if got = SecretKeySelectorMask(nil); got != nil {
		t.Errorf("SecretKeySelectorMask(nil) = %v, want: nil", got)
	}
}

func TestConfigMapEnvSourceMask(t *testing.T) {
	want := &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{},
		Optional:             ptr.Bool(true),
	}
	in := want

	got := ConfigMapEnvSourceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ConfigMapEnvSourceMask (-want, +got): %s", diff)
	}

	if got = ConfigMapEnvSourceMask(nil); got != nil {
		t.Errorf("ConfigMapEnvSourceMask(nil) = %v, want: nil", got)
	}
}

func TestSecretEnvSourceMask(t *testing.T) {
	want := &corev1.SecretEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{},
		Optional:             ptr.Bool(true),
	}
	in := want

	got := SecretEnvSourceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("SecretEnvSourceMask (-want, +got): %s", diff)
	}

	if got = SecretEnvSourceMask(nil); got != nil {
		t.Errorf("SecretEnvSourceMask(nil) = %v, want: nil", got)
	}
}

func TestEnvFromSourceMask(t *testing.T) {
	want := &corev1.EnvFromSource{
		Prefix:       "foo",
		ConfigMapRef: &corev1.ConfigMapEnvSource{},
		SecretRef:    &corev1.SecretEnvSource{},
	}
	in := want

	got := EnvFromSourceMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("EnvFromSourceMask (-want, +got): %s", diff)
	}

	if got = EnvFromSourceMask(nil); got != nil {
		t.Errorf("EnvFromSourceMask(nil) = %v, want: nil", got)
	}
}

func TestResourceRequirementsMask(t *testing.T) {
	want := &corev1.ResourceRequirements{
		Limits:   make(corev1.ResourceList),
		Requests: make(corev1.ResourceList),
	}
	in := want

	got := ResourceRequirementsMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("ResourceRequirementsMask (-want, +got): %s", diff)
	}

	if got = ResourceRequirementsMask(nil); got != nil {
		t.Errorf("ResourceRequirementsMask(nil) = %v, want: nil", got)
	}
}

func TestSecurityContextMask(t *testing.T) {
	mtype := corev1.UnmaskedProcMount
	want := &corev1.SecurityContext{
		RunAsUser: ptr.Int64(1),
	}
	in := &corev1.SecurityContext{
		RunAsUser:                ptr.Int64(1),
		Capabilities:             &corev1.Capabilities{},
		Privileged:               ptr.Bool(true),
		SELinuxOptions:           &corev1.SELinuxOptions{},
		RunAsGroup:               ptr.Int64(2),
		RunAsNonRoot:             ptr.Bool(true),
		ReadOnlyRootFilesystem:   ptr.Bool(true),
		AllowPrivilegeEscalation: ptr.Bool(true),
		ProcMount:                &mtype,
	}

	got := SecurityContextMask(in)

	if &want == &got {
		t.Errorf("Input and output share addresses. Want different addresses")
	}

	if diff, err := kmp.SafeDiff(want, got); err != nil {
		t.Errorf("Got error comparing output, err = %v", err)
	} else if diff != "" {
		t.Errorf("SecurityContextMask (-want, +got): %s", diff)
	}

	if got = SecurityContextMask(nil); got != nil {
		t.Errorf("SecurityContextMask(nil) = %v, want: nil", got)
	}
}
