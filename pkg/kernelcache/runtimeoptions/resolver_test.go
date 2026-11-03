/*
Copyright 2026 The KServe Authors.

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

package runtimeoptions

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestResolvePrecedenceAndCLIForms(t *testing.T) {
	container := &corev1.Container{
		Env:     []corev1.EnvVar{{Name: "RUNTIME_DTYPE", Value: "float16"}},
		Args:    []string{"--dtype", "bfloat16", "--tp=2", "--tp", "4"},
		Command: []string{"runtime", "--dtype=fp8", "--tp", "8"},
	}

	got := Resolve(container, []Spec{
		{Key: "dtype", Env: []string{"RUNTIME_DTYPE"}, Flags: []string{"--dtype"}},
		{Key: "tensor-parallel-size", Flags: []string{"--tensor-parallel-size", "--tp"}},
	})

	want := map[string]string{
		"dtype":                "bfloat16",
		"tensor-parallel-size": "4",
	}
	assertStringMapEqual(t, want, got)
}

func TestResolvePreservesTokenizedValues(t *testing.T) {
	container := &corev1.Container{
		Args: []string{"--compilation-config", `{"level": 3, "mode": "max-autotune"}`},
	}

	got := Resolve(container, []Spec{{Key: "compilation-config", Flags: []string{"--compilation-config"}}})

	assertStringMapEqual(t, map[string]string{
		"compilation-config": `{"level": 3, "mode": "max-autotune"}`,
	}, got)
}

func TestResolveBooleanOptions(t *testing.T) {
	container := &corev1.Container{
		Args: []string{"--enable-feature", "--no-other-feature", "--other-feature=true"},
	}

	got := Resolve(container, []Spec{
		{Key: "feature", Flags: []string{"--enable-feature"}, Boolean: true},
		{Key: "other-feature", Flags: []string{"--other-feature"}, Boolean: true},
	})

	assertStringMapEqual(t, map[string]string{
		"feature":       "true",
		"other-feature": "true",
	}, got)

	got = Resolve(&corev1.Container{Args: []string{"--no-feature"}}, []Spec{
		{Key: "feature", Flags: []string{"--feature"}, Boolean: true},
	})
	assertStringMapEqual(t, map[string]string{"feature": "false"}, got)
}

func TestResolveShellCommandAndEnvironmentExpansion(t *testing.T) {
	container := &corev1.Container{
		Env:  []corev1.EnvVar{{Name: "RUNTIME_ADDITIONAL_ARGS", Value: "--dtype float16 --tp 4"}},
		Args: []string{"--tp", "2"},
		Command: []string{
			"/bin/bash",
			"-c",
			`eval "exec runtime serve ${RUNTIME_ADDITIONAL_ARGS} --dtype bfloat16 $@"`,
		},
	}

	got := Resolve(container, []Spec{
		{Key: "dtype", Flags: []string{"--dtype"}},
		{Key: "tensor-parallel-size", Flags: []string{"--tp"}},
	})

	assertStringMapEqual(t, map[string]string{
		"dtype":                "bfloat16",
		"tensor-parallel-size": "2",
	}, got)
}

func TestResolveShellCommandIgnoresLongFlagsContainingC(t *testing.T) {
	container := &corev1.Container{
		Command: []string{
			"/bin/bash",
			"--rcfile",
			"/etc/bashrc",
			"-c",
			"exec runtime --dtype float16",
		},
	}

	got := Resolve(container, []Spec{{Key: "dtype", Flags: []string{"--dtype"}}})

	assertStringMapEqual(t, map[string]string{"dtype": "float16"}, got)
}

func TestResolveDoesNotExpandSingleQuotedReferences(t *testing.T) {
	container := &corev1.Container{
		Env:     []corev1.EnvVar{{Name: "RUNTIME_DTYPE", Value: "float16"}},
		Command: []string{"/bin/sh", "-c", `exec runtime --dtype '$RUNTIME_DTYPE'`},
	}

	got := Resolve(container, []Spec{{Key: "dtype", Flags: []string{"--dtype"}}})
	assertStringMapEqual(t, map[string]string{}, got)
}

func TestResolveIgnoresIndirectEnvironmentValues(t *testing.T) {
	container := &corev1.Container{
		Env: []corev1.EnvVar{{
			Name: "RUNTIME_DTYPE",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{},
			},
		}},
	}

	got := Resolve(container, []Spec{{Key: "dtype", Env: []string{"RUNTIME_DTYPE"}}})
	assertStringMapEqual(t, map[string]string{}, got)
}

func assertStringMapEqual(t *testing.T, want, got map[string]string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("expected %s=%q, got %q in %v", key, value, got[key], got)
		}
	}
}
