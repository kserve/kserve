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

package llmisvc

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	kservevalidation "github.com/kserve/kserve/pkg/validation"
)

func mxService(t *testing.T, uri string, annotations map[string]string) *v1alpha2.LLMInferenceService {
	t.Helper()
	u, err := apis.ParseURL(uri)
	require.NoError(t, err)
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ns", Annotations: annotations},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *u},
		},
	}
}

// presetPod mirrors the main container the shipped vLLM presets render.
func presetPod() *corev1.PodSpec {
	return &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: mainContainerName,
			Args: []string{"--"},
			Env: []corev1.EnvVar{
				{Name: kvTransferArgsEnvVar, Value: ""},
				{Name: modelArgsEnvVar, Value: ""},
				{Name: "HOME", Value: "/home"},
				{Name: "HF_HUB_CACHE", Value: "/models"},
			},
			VolumeMounts: []corev1.VolumeMount{{Name: "model-cache", MountPath: "/models"}},
		}},
		Volumes: []corev1.Volume{{Name: "model-cache", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
	}
}

func mxLayered() *modelExpressConfig {
	return &modelExpressConfig{
		Mode:     kservevalidation.ModelExpressModeLayered,
		Server:   modelExpressServer{Address: "https://mx.mx.svc:8001", TokenAudience: "modelexpress"},
		Revision: "r1",
	}
}

func mxNative() *modelExpressConfig {
	mx := mxLayered()
	mx.Mode = kservevalidation.ModelExpressModeNative
	return mx
}

func envOf(c *corev1.Container, name string) (corev1.EnvVar, bool) {
	for _, e := range c.Env {
		if e.Name == name {
			return e, true
		}
	}
	return corev1.EnvVar{}, false
}

func envCount(c *corev1.Container, name string) int {
	n := 0
	for _, e := range c.Env {
		if e.Name == name {
			n++
		}
	}
	return n
}

func mainOf(t *testing.T, podSpec *corev1.PodSpec) *corev1.Container {
	t.Helper()
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == mainContainerName {
			return &podSpec.Containers[i]
		}
	}
	require.FailNow(t, "main container not found")
	return nil
}

func TestAttachModelExpressDisabled(t *testing.T) {
	t.Parallel()

	t.Run("removes the empty model slot and changes nothing else", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		expected := presetPod()
		expected.Containers[0].Env = []corev1.EnvVar{
			{Name: kvTransferArgsEnvVar, Value: ""},
			{Name: "HOME", Value: "/home"},
			{Name: "HF_HUB_CACHE", Value: "/models"},
		}

		require.NoError(t, attachModelExpress(mxService(t, "hf://org/model", nil), podSpec, nil))
		assert.Equal(t, expected, podSpec)
	})

	t.Run("leaves presets without the slot untouched", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		podSpec.Containers[0].Env = podSpec.Containers[0].Env[:1]
		expected := podSpec.DeepCopy()

		require.NoError(t, attachModelExpress(mxService(t, "s3://b/m", nil), podSpec, nil))
		assert.Equal(t, expected, podSpec)
	})

	t.Run("ignores pods without a main container", func(t *testing.T) {
		t.Parallel()
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "sidecar"}}}
		expected := podSpec.DeepCopy()

		require.NoError(t, attachModelExpress(mxService(t, "s3://b/m", nil), podSpec, mxNative()))
		assert.Equal(t, expected, podSpec)
	})
}

func TestAttachModelExpressLayered(t *testing.T) {
	t.Parallel()

	podSpec := presetPod()
	require.NoError(t, attachModelExpress(mxService(t, "pvc://models/llama", nil), podSpec, mxLayered()))
	c := mainOf(t, podSpec)

	assert.Equal(t, []string{"--", "--load-format", "modelexpress"}, c.Args)
	_, hasSlot := envOf(c, modelArgsEnvVar)
	assert.False(t, hasSlot, "layered mode serves /mnt/models, so the slot is removed")

	for name, value := range map[string]string{
		"MX_SERVER_ADDRESS":  "https://mx.mx.svc:8001",
		"MODEL_EXPRESS_URL":  "https://mx.mx.svc:8001",
		"MX_MODEL_REVISION":  "r1",
		"MX_AUTH_TOKEN_PATH": "/var/run/secrets/modelexpress/token",
	} {
		e, ok := envOf(c, name)
		require.True(t, ok, name)
		assert.Equal(t, value, e.Value, name)
	}
	for name, fieldPath := range map[string]string{
		"POD_NAME":       "metadata.name",
		"POD_NAMESPACE":  "metadata.namespace",
		"POD_UID":        "metadata.uid",
		"MX_WORKER_HOST": "status.podIP",
	} {
		e, ok := envOf(c, name)
		require.True(t, ok, name)
		require.NotNil(t, e.ValueFrom, name)
		assert.Equal(t, fieldPath, e.ValueFrom.FieldRef.FieldPath, name)
	}
	for _, name := range []string{"MX_MODEL_URI", "MODEL_EXPRESS_NO_SHARED_STORAGE", "HF_HUB_OFFLINE", "MODEL_EXPRESS_CACHE_DIRECTORY"} {
		_, ok := envOf(c, name)
		assert.False(t, ok, "%s is native-only", name)
	}

	assert.Contains(t, podSpec.Volumes, corev1.Volume{
		Name: modelExpressTokenVolumeName,
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
			Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
				Audience:          "modelexpress",
				ExpirationSeconds: ptr.To(int64(3600)),
				Path:              "token",
			}}},
		}},
	})
	assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: modelExpressTokenVolumeName, MountPath: "/var/run/secrets/modelexpress", ReadOnly: true})
	assert.Nil(t, c.SecurityContext, "no capabilities are added")
}

func TestAttachModelExpressRespectsWorkload(t *testing.T) {
	t.Parallel()

	t.Run("keeps a user --load-format", func(t *testing.T) {
		t.Parallel()
		for _, args := range [][]string{{"--", "--load-format", "runai_streamer"}, {"--", "--load-format=runai_streamer"}} {
			podSpec := presetPod()
			podSpec.Containers[0].Args = append([]string(nil), args...)
			require.NoError(t, attachModelExpress(mxService(t, "s3://b/m", nil), podSpec, mxLayered()))
			assert.Equal(t, args, mainOf(t, podSpec).Args)
		}
	})

	t.Run("keeps user-set env", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{Name: "MX_WORKER_HOST", Value: "10.0.0.9"})
		require.NoError(t, attachModelExpress(mxService(t, "s3://b/m", nil), podSpec, mxLayered()))
		c := mainOf(t, podSpec)
		e, _ := envOf(c, "MX_WORKER_HOST")
		assert.Equal(t, corev1.EnvVar{Name: "MX_WORKER_HOST", Value: "10.0.0.9"}, e)
		assert.Equal(t, 1, envCount(c, "MX_WORKER_HOST"))
	})

	t.Run("sends no token without an audience", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		mx := mxLayered()
		mx.Server.TokenAudience = ""
		require.NoError(t, attachModelExpress(mxService(t, "s3://b/m", nil), podSpec, mx))
		c := mainOf(t, podSpec)
		_, ok := envOf(c, "MX_AUTH_TOKEN_PATH")
		assert.False(t, ok)
		for _, v := range podSpec.Volumes {
			assert.NotEqual(t, modelExpressTokenVolumeName, v.Name)
		}
	})

	t.Run("is idempotent on the same pod spec", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		svc := mxService(t, "hf://org/model", nil)
		require.NoError(t, attachModelExpress(svc, podSpec, mxLayered()))
		once := podSpec.DeepCopy()
		require.NoError(t, attachModelExpress(svc, podSpec, mxLayered()))
		assert.Equal(t, once, podSpec)
	})
}

func TestAttachModelExpressNativeHuggingFace(t *testing.T) {
	t.Parallel()

	t.Run("fills the slot and reuses the preset Hugging Face cache", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		require.NoError(t, attachModelExpress(mxService(t, "hf://meta-llama/Llama-3.3-70B-Instruct", nil), podSpec, mxNative()))
		c := mainOf(t, podSpec)

		assert.Equal(t, corev1.EnvVar{Name: modelArgsEnvVar, Value: "meta-llama/Llama-3.3-70B-Instruct"}, c.Env[1], "slot keeps its position")
		for name, value := range map[string]string{
			"MODEL_EXPRESS_NO_SHARED_STORAGE": "1",
			"MODEL_EXPRESS_CACHE_DIRECTORY":   "/models",
			"HF_HUB_OFFLINE":                  "1",
			"HF_HUB_CACHE":                    "/models",
		} {
			e, ok := envOf(c, name)
			require.True(t, ok, name)
			assert.Equal(t, value, e.Value, name)
		}
		assert.Len(t, podSpec.Volumes, 2, "model-cache plus the token volume")
		_, ok := envOf(c, "MX_MODEL_URI")
		assert.False(t, ok)
	})

	t.Run("passes the revision to vLLM", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		require.NoError(t, attachModelExpress(mxService(t, "hf://org/model:abc123", nil), podSpec, mxNative()))
		e, _ := envOf(mainOf(t, podSpec), modelArgsEnvVar)
		assert.Equal(t, "org/model --revision abc123", e.Value)
	})

	t.Run("adds a cache volume when the preset has no Hugging Face cache", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		podSpec.Containers[0].Env = podSpec.Containers[0].Env[:2]
		require.NoError(t, attachModelExpress(mxService(t, "hf://org/model", nil), podSpec, mxNative()))
		c := mainOf(t, podSpec)

		for _, name := range []string{"HF_HUB_CACHE", "MODEL_EXPRESS_CACHE_DIRECTORY"} {
			e, ok := envOf(c, name)
			require.True(t, ok, name)
			assert.Equal(t, modelExpressCacheMountPath, e.Value, name)
		}
		assert.Contains(t, podSpec.Volumes, corev1.Volume{Name: modelExpressCacheVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
		assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: modelExpressCacheVolumeName, MountPath: modelExpressCacheMountPath})
	})

	t.Run("adds a cache volume when HF_HUB_CACHE comes from a reference", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		podSpec.Containers[0].Env[3] = corev1.EnvVar{Name: "HF_HUB_CACHE", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "k"}}}
		require.NoError(t, attachModelExpress(mxService(t, "hf://org/model", nil), podSpec, mxNative()))
		e, _ := envOf(mainOf(t, podSpec), "MODEL_EXPRESS_CACHE_DIRECTORY")
		assert.Equal(t, modelExpressCacheMountPath, e.Value)
	})

	t.Run("rejects a malformed Hugging Face URI", func(t *testing.T) {
		t.Parallel()
		err := attachModelExpress(mxService(t, "hf://just-a-name", nil), presetPod(), mxNative())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hf://owner/model[:revision]")
	})

	t.Run("native s3 leaves the slot empty", func(t *testing.T) {
		t.Parallel()
		podSpec := presetPod()
		require.NoError(t, attachModelExpress(mxService(t, "s3://bucket/llama", nil), podSpec, mxNative()))
		c := mainOf(t, podSpec)
		_, ok := envOf(c, modelArgsEnvVar)
		assert.False(t, ok)
		_, ok = envOf(c, "MODEL_EXPRESS_NO_SHARED_STORAGE")
		assert.False(t, ok)
	})
}

func TestFillModelArgsSlot(t *testing.T) {
	t.Parallel()

	t.Run("rejects a user value", func(t *testing.T) {
		t.Parallel()
		c := &corev1.Container{Name: "main", Env: []corev1.EnvVar{{Name: modelArgsEnvVar, Value: "/my/model"}}}
		var slotErr *modelArgsSlotError
		require.ErrorAs(t, fillModelArgsSlot(c, ""), &slotErr)
		assert.Equal(t, "main", slotErr.container)
	})

	t.Run("rejects a reference", func(t *testing.T) {
		t.Parallel()
		c := &corev1.Container{Name: "main", Env: []corev1.EnvVar{{Name: modelArgsEnvVar, ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}}}}
		assert.Error(t, fillModelArgsSlot(c, "'org/model'"))
	})

	t.Run("rejects a user value on a duplicate slot", func(t *testing.T) {
		t.Parallel()
		c := &corev1.Container{Name: "main", Env: []corev1.EnvVar{{Name: modelArgsEnvVar}, {Name: modelArgsEnvVar, Value: "/my/model"}}}
		assert.Error(t, fillModelArgsSlot(c, ""))
	})

	t.Run("collapses duplicates onto the first position", func(t *testing.T) {
		t.Parallel()
		c := &corev1.Container{Env: []corev1.EnvVar{{Name: "A"}, {Name: modelArgsEnvVar}, {Name: "B"}, {Name: modelArgsEnvVar}}}
		require.NoError(t, fillModelArgsSlot(c, "'org/model'"))
		assert.Equal(t, []corev1.EnvVar{{Name: "A"}, {Name: modelArgsEnvVar, Value: "'org/model'"}, {Name: "B"}}, c.Env)
	})
}

func TestHfModelArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uri     string
		want    string
		wantErr bool
	}{
		{uri: "hf://org/model", want: "org/model"},
		{uri: "hf://org/model:main", want: "org/model --revision main"},
		{uri: "hf://org/model.v2-instruct:0123abc", want: "org/model.v2-instruct --revision 0123abc"},
		{uri: "hf://org/it's", want: `'org/it'\''s'`},
		{uri: "hf://model", wantErr: true},
		{uri: "hf://org/", wantErr: true},
		{uri: "hf:///model", wantErr: true},
		{uri: "hf://org/model/extra", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			t.Parallel()
			got, err := hfModelArgs(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func objectStorePod(initEnv []corev1.EnvVar, initMounts []corev1.VolumeMount) *corev1.PodSpec {
	podSpec := presetPod()
	podSpec.InitContainers = []corev1.Container{{
		Name:         constants.StorageInitializerContainerName,
		Args:         []string{"s3://bucket/llama", constants.DefaultModelLocalMountPath},
		Env:          slices.Clone(initEnv),
		VolumeMounts: slices.Clone(initMounts),
	}}
	return podSpec
}

func TestAttachModelExpressObjectStore(t *testing.T) {
	t.Parallel()

	credentials := []corev1.EnvVar{
		{Name: "AWS_ACCESS_KEY_ID", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "s3"}, Key: "AWS_ACCESS_KEY_ID"}}},
		{Name: "AWS_SECRET_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "s3"}, Key: "AWS_SECRET_ACCESS_KEY"}}},
		{Name: "S3_ENDPOINT", Value: "minio.minio.svc:9000"},
		{Name: "AWS_ENDPOINT_URL", Value: "http://minio.minio.svc:9000"},
		{Name: "S3_USE_HTTPS", Value: "0"},
		{Name: "STORAGE_CONFIG", Value: "unrelated"},
	}

	t.Run("downloads non-weight files and streams weights with the init container credentials", func(t *testing.T) {
		t.Parallel()
		podSpec := objectStorePod(credentials, nil)
		require.NoError(t, attachModelExpressObjectStore(podSpec, mainContainerName, "s3://bucket/llama"))

		ignore, ok := envOf(&podSpec.InitContainers[0], "STORAGE_IGNORE_PATTERNS")
		require.True(t, ok)
		assert.JSONEq(t, `["*.safetensors", "*.bin", "*.pt", "*.pth", "*.gguf", "*.h5", "*.msgpack", "*.onnx"]`, ignore.Value)

		c := mainOf(t, podSpec)
		for _, want := range credentials[:5] {
			got, ok := envOf(c, want.Name)
			require.True(t, ok, want.Name)
			assert.Equal(t, want, got)
		}
		_, ok = envOf(c, "STORAGE_CONFIG")
		assert.False(t, ok, "only AWS_ and S3_ env is shared")

		for name, value := range map[string]string{
			"MX_MODEL_URI": "s3://bucket/llama",
			"RUNAI_STREAMER_S3_USE_VIRTUAL_ADDRESSING": "0",
			"AWS_EC2_METADATA_DISABLED":                "true",
		} {
			e, ok := envOf(c, name)
			require.True(t, ok, name)
			assert.Equal(t, value, e.Value, name)
		}
	})

	t.Run("honors virtual-hosted buckets", func(t *testing.T) {
		t.Parallel()
		env := append(append([]corev1.EnvVar(nil), credentials...), corev1.EnvVar{Name: "S3_USER_VIRTUAL_BUCKET", Value: "True"})
		podSpec := objectStorePod(env, nil)
		require.NoError(t, attachModelExpressObjectStore(podSpec, mainContainerName, "s3://bucket/llama"))
		e, _ := envOf(mainOf(t, podSpec), "RUNAI_STREAMER_S3_USE_VIRTUAL_ADDRESSING")
		assert.Equal(t, "1", e.Value)
	})

	t.Run("leaves addressing to the streamer on AWS", func(t *testing.T) {
		t.Parallel()
		podSpec := objectStorePod(credentials[:2], nil)
		require.NoError(t, attachModelExpressObjectStore(podSpec, mainContainerName, "s3://bucket/llama"))
		c := mainOf(t, podSpec)
		for _, name := range []string{"RUNAI_STREAMER_S3_USE_VIRTUAL_ADDRESSING", "AWS_EC2_METADATA_DISABLED", "AWS_CA_BUNDLE"} {
			_, ok := envOf(c, name)
			assert.False(t, ok, name)
		}
	})

	t.Run("shares the CA bundle mount", func(t *testing.T) {
		t.Parallel()
		mount := corev1.VolumeMount{Name: CaBundleVolumeName, MountPath: "/etc/ssl/custom-certs", ReadOnly: true}
		podSpec := objectStorePod(credentials, []corev1.VolumeMount{mount})
		require.NoError(t, attachModelExpressObjectStore(podSpec, mainContainerName, "s3://bucket/llama"))
		c := mainOf(t, podSpec)
		assert.Contains(t, c.VolumeMounts, mount)
		e, _ := envOf(c, "AWS_CA_BUNDLE")
		assert.Equal(t, "/etc/ssl/custom-certs/cabundle.crt", e.Value)
	})

	t.Run("keeps an explicit CA bundle path", func(t *testing.T) {
		t.Parallel()
		env := append(append([]corev1.EnvVar(nil), credentials...), corev1.EnvVar{Name: "AWS_CA_BUNDLE", Value: "/etc/ssl/custom-certs/minio.crt"})
		mount := corev1.VolumeMount{Name: CaBundleVolumeName, MountPath: "/etc/ssl/custom-certs", ReadOnly: true}
		podSpec := objectStorePod(env, []corev1.VolumeMount{mount})
		require.NoError(t, attachModelExpressObjectStore(podSpec, mainContainerName, "s3://bucket/llama"))
		c := mainOf(t, podSpec)
		e, _ := envOf(c, "AWS_CA_BUNDLE")
		assert.Equal(t, "/etc/ssl/custom-certs/minio.crt", e.Value)
		assert.Equal(t, 1, envCount(c, "AWS_CA_BUNDLE"))
	})

	t.Run("fails without a storage initializer", func(t *testing.T) {
		t.Parallel()
		err := attachModelExpressObjectStore(presetPod(), mainContainerName, "s3://bucket/llama")
		assert.ErrorContains(t, err, "storage-initializer")
	})
}

func TestModelExpressRevision(t *testing.T) {
	t.Parallel()

	engine := func() *corev1.PodSpec {
		return &corev1.PodSpec{Containers: []corev1.Container{{
			Name:    mainContainerName,
			Image:   "vllm:v1",
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{"vllm serve /mnt/models --kv-cache-dtype fp8"},
			Env:     []corev1.EnvVar{{Name: "VLLM_LOGGING_LEVEL", Value: "INFO"}},
		}}}
	}
	service := func(t *testing.T) *v1alpha2.LLMInferenceService {
		svc := mxService(t, "s3://bucket/llama", nil)
		svc.Spec.Template = engine()
		svc.Spec.Worker = engine()
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{Template: engine(), Worker: engine()}
		return svc
	}
	revision := func(t *testing.T, svc *v1alpha2.LLMInferenceService) string {
		t.Helper()
		rev, err := modelExpressRevision(svc)
		require.NoError(t, err)
		return rev
	}
	base := revision(t, service(t))

	t.Run("is a stable digest", func(t *testing.T) {
		t.Parallel()
		assert.Regexp(t, `^spec-[0-9a-f]{16}$`, base)
		assert.Equal(t, base, revision(t, service(t)))
	})

	t.Run("the annotation overrides the digest", func(t *testing.T) {
		t.Parallel()
		svc := service(t)
		svc.Annotations = map[string]string{constants.ModelExpressRevisionAnnotationKey: " 2026-09-01 "}
		assert.Equal(t, "2026-09-01", revision(t, svc))
	})

	t.Run("a blank annotation keeps the digest", func(t *testing.T) {
		t.Parallel()
		svc := service(t)
		svc.Annotations = map[string]string{constants.ModelExpressRevisionAnnotationKey: "  "}
		assert.Equal(t, base, revision(t, svc))
	})

	t.Run("changes with the model URI", func(t *testing.T) {
		t.Parallel()
		svc := service(t)
		u, err := apis.ParseURL("s3://bucket/mistral")
		require.NoError(t, err)
		svc.Spec.Model.URI = *u
		assert.NotEqual(t, base, revision(t, svc))
	})

	roles := map[string]func(*v1alpha2.LLMInferenceService) *corev1.PodSpec{
		"template":         func(s *v1alpha2.LLMInferenceService) *corev1.PodSpec { return s.Spec.Template },
		"worker":           func(s *v1alpha2.LLMInferenceService) *corev1.PodSpec { return s.Spec.Worker },
		"prefill template": func(s *v1alpha2.LLMInferenceService) *corev1.PodSpec { return s.Spec.Prefill.Template },
		"prefill worker":   func(s *v1alpha2.LLMInferenceService) *corev1.PodSpec { return s.Spec.Prefill.Worker },
	}
	changes := map[string]func(*corev1.Container){
		"image":   func(c *corev1.Container) { c.Image = "vllm:v2" },
		"command": func(c *corev1.Container) { c.Command = []string{"/bin/bash", "-c"} },
		"args":    func(c *corev1.Container) { c.Args = []string{"vllm serve /mnt/models --kv-cache-dtype auto"} },
		"env value": func(c *corev1.Container) {
			c.Env = []corev1.EnvVar{{Name: "VLLM_LOGGING_LEVEL", Value: "DEBUG"}}
		},
		"env added": func(c *corev1.Container) {
			c.Env = append(c.Env, corev1.EnvVar{Name: "VLLM_USE_DEEP_GEMM", Value: "1"})
		},
		"env from secret": func(c *corev1.Container) {
			c.Env = append(c.Env, corev1.EnvVar{Name: "HF_TOKEN", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "hf"}, Key: "token"},
			}})
		},
	}
	for role, podSpec := range roles {
		for field, change := range changes {
			t.Run("changes with the "+role+" main container "+field, func(t *testing.T) {
				t.Parallel()
				svc := service(t)
				change(&podSpec(svc).Containers[0])
				assert.NotEqual(t, base, revision(t, svc))
			})
		}
	}

	t.Run("moving the same container to another role changes it", func(t *testing.T) {
		t.Parallel()
		svc := service(t)
		svc.Spec.Prefill = nil
		withoutPrefill := revision(t, svc)
		assert.NotEqual(t, base, withoutPrefill)

		svc.Spec.Worker = nil
		assert.NotEqual(t, withoutPrefill, revision(t, svc))
	})

	ignored := map[string]func(*v1alpha2.LLMInferenceService){
		"replicas": func(s *v1alpha2.LLMInferenceService) { s.Spec.Replicas = ptr.To(int32(4)) },
		"labels":   func(s *v1alpha2.LLMInferenceService) { s.Spec.Labels = map[string]string{"team": "a"} },
		"annotations": func(s *v1alpha2.LLMInferenceService) {
			s.Annotations = map[string]string{"example.com/note": "x"}
		},
		"sidecars": func(s *v1alpha2.LLMInferenceService) {
			s.Spec.Template.Containers = append(s.Spec.Template.Containers, corev1.Container{Name: "proxy", Image: "proxy:v1"})
		},
		"main container resources": func(s *v1alpha2.LLMInferenceService) {
			s.Spec.Template.Containers[0].Resources.Limits = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Gi")}
		},
		"node selector": func(s *v1alpha2.LLMInferenceService) {
			s.Spec.Template.NodeSelector = map[string]string{"gpu": "h100"}
		},
	}
	for name, change := range ignored {
		t.Run("ignores "+name, func(t *testing.T) {
			t.Parallel()
			svc := service(t)
			change(svc)
			assert.Equal(t, base, revision(t, svc))
		})
	}

	t.Run("a service without engine templates hashes the model URI", func(t *testing.T) {
		t.Parallel()
		a := revision(t, mxService(t, "s3://bucket/llama", nil))
		assert.Regexp(t, `^spec-[0-9a-f]{16}$`, a)
		assert.NotEqual(t, a, revision(t, mxService(t, "s3://bucket/mistral", nil)))
	})
}

func TestModelExpressServerFromAnnotations(t *testing.T) {
	t.Parallel()

	_, err := modelExpressServerFromAnnotations(mxService(t, "s3://b/m", nil))
	assert.ErrorContains(t, err, constants.ModelExpressAddressAnnotationKey)

	_, err = modelExpressServerFromAnnotations(mxService(t, "s3://b/m", map[string]string{constants.ModelExpressAddressAnnotationKey: "grpc://mx:8001"}))
	assert.ErrorContains(t, err, "unsupported scheme")

	server, err := modelExpressServerFromAnnotations(mxService(t, "s3://b/m", map[string]string{
		constants.ModelExpressAddressAnnotationKey:       " mx.mx.svc:8001 ",
		constants.ModelExpressTokenAudienceAnnotationKey: "modelexpress",
	}))
	require.NoError(t, err)
	assert.Equal(t, &modelExpressServer{Address: "mx.mx.svc:8001", TokenAudience: "modelexpress"}, server)
}

func TestValidateNativeModelExpressWorkload(t *testing.T) {
	t.Parallel()

	withTemplates := func(svc *v1alpha2.LLMInferenceService, prefillWorker *corev1.PodSpec) *v1alpha2.LLMInferenceService {
		svc.Spec.Template = presetPod()
		svc.Spec.Worker = presetPod()
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{Template: presetPod(), Worker: prefillWorker}
		return svc
	}
	noSlot := presetPod()
	noSlot.Containers[0].Env = noSlot.Containers[0].Env[:1]

	assert.NoError(t, validateNativeModelExpressWorkload(withTemplates(mxService(t, "hf://org/model", nil), presetPod())))

	err := validateNativeModelExpressWorkload(withTemplates(mxService(t, "hf://org/model", nil), noSlot))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prefill worker")
	assert.Contains(t, err.Error(), modelArgsEnvVar)

	assert.NoError(t, validateNativeModelExpressWorkload(withTemplates(mxService(t, "s3://bucket/llama", nil), noSlot)),
		"s3:// serves /mnt/models and needs no slot")

	disabled := mxService(t, "s3://bucket/llama", nil)
	disabled.Spec.StorageInitializer = &v1alpha2.StorageInitializerSpec{Enabled: ptr.To(false)}
	assert.ErrorContains(t, validateNativeModelExpressWorkload(disabled), "spec.storageInitializer.enabled")
}
