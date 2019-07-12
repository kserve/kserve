// +build e2e

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

package conformance

import (
	"path/filepath"
	"testing"

	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
	"github.com/knative/serving/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestConfigMapVolume tests that we echo back the appropriate text from the ConfigMap volume.
func TestConfigMapVolume(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   "hellovolume",
	}

	text := test.AppendRandomString("hello-volumes-")

	// Create the ConfigMap with random text.
	configMap, err := clients.KubeClient.Kube.CoreV1().ConfigMaps(test.ServingNamespace).Create(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.Service, // Give it the same name as the service.
		},
		Data: map[string]string{
			filepath.Base(test.HelloVolumePath): text,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create configmap: %v", err)
	}
	t.Logf("Successfully created configMap: %v", configMap)

	cleanup := func() {
		test.TearDown(clients, names)
		if err := clients.KubeClient.Kube.CoreV1().ConfigMaps(test.ServingNamespace).Delete(configMap.Name, nil); err != nil {
			t.Errorf("ConfigMaps().Delete() = %v", err)
		}
	}

	// Clean up on test failure or interrupt
	defer cleanup()
	test.CleanupOnInterrupt(cleanup)

	withVolume := WithVolume("asdf", filepath.Dir(test.HelloVolumePath), corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configMap.Name,
			},
		},
	},
	)

	// Setup initial Service
	if _, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{}, withVolume); err != nil {

		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}

	// Validate State after Creation

	if err = validateRunLatestControlPlane(t, clients, names, "1"); err != nil {
		t.Error(err)
	}

	if err = validateRunLatestDataPlane(t, clients, names, text); err != nil {
		t.Error(err)
	}
}

// TestSecretVolume tests that we echo back the appropriate text from the Secret volume.
func TestSecretVolume(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   "hellovolume",
	}

	text := test.ObjectNameForTest(t)

	// Create the Secret with random text.
	secret, err := clients.KubeClient.Kube.CoreV1().Secrets(test.ServingNamespace).Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.Service, // name the Secret the same as the Service.
		},
		StringData: map[string]string{
			filepath.Base(test.HelloVolumePath): text,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	t.Logf("Successfully created secret: %v", secret)

	cleanup := func() {
		test.TearDown(clients, names)
		if err := clients.KubeClient.Kube.CoreV1().Secrets(test.ServingNamespace).Delete(secret.Name, nil); err != nil {
			t.Errorf("Secrets().Delete() = %v", err)
		}
	}

	// Clean up on test failure or interrupt
	defer cleanup()
	test.CleanupOnInterrupt(cleanup)

	withVolume := WithVolume("asdf", filepath.Dir(test.HelloVolumePath), corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: secret.Name,
		},
	})

	// Setup initial Service
	if _, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{}, withVolume); err != nil {

		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}

	// Validate State after Creation

	if err = validateRunLatestControlPlane(t, clients, names, "1"); err != nil {
		t.Error(err)
	}

	if err = validateRunLatestDataPlane(t, clients, names, text); err != nil {
		t.Error(err)
	}
}
