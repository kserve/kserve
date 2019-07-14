// +build e2e

/*
Copyright 2018 The Knative Authors

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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/knative/serving/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testKey   = "testKey"
	testValue = "testValue"
)

// TestSecretsFromEnv verifies propagation of Secrets through environment variables.
func TestSecretsFromEnv(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	secretName := test.AppendRandomString("secret-")

	//Creating test secret
	secret, err := clients.KubeClient.Kube.CoreV1().Secrets(test.ServingNamespace).Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			testKey: testValue,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Successfully created test secret: %v", secret)

	err = fetchEnvironmentAndVerify(t, clients, corev1.EnvVar{
		Name: testKey,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: testKey,
			},
		},
	}, cleanupSecret(secretName))
	if err != nil {
		t.Fatal(err)
	}
}

// TestConfigsFromEnv verifies propagation of configs through environment variables.
func TestConfigsFromEnv(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	configMapName := test.AppendRandomString("configmap-")

	//Creating test configMap
	configMap, err := clients.KubeClient.Kube.CoreV1().ConfigMaps(test.ServingNamespace).Create(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			testKey: testValue,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Successfully created configMap: %v", configMap)

	err = fetchEnvironmentAndVerify(t, clients, corev1.EnvVar{
		Name: testKey,
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
				Key: testKey,
			},
		},
	}, cleanupConfigMap(configMapName))
	if err != nil {
		t.Fatal(err)
	}
}

func fetchEnvironmentAndVerify(t *testing.T, clients *test.Clients, envVar corev1.EnvVar, cleanup func(clients *test.Clients) error) error {
	resp, _, err := fetchEnvInfo(t, clients, test.EnvImageEnvVarsPath, &test.Options{
		EnvVars: []corev1.EnvVar{envVar},
	})
	if err != nil {
		cleanupError := cleanup(clients)
		t.Error(cleanupError)
		return err
	}

	var envVars map[string]string
	err = json.Unmarshal(resp, &envVars)
	if err != nil {
		if cleanupError := cleanup(clients); cleanupError != nil {
			t.Error(cleanupError)
		}
		return err
	}

	if value, ok := envVars[testKey]; ok {
		if value != testValue {
			if cleanupError := cleanup(clients); cleanupError != nil {
				t.Error(cleanupError)
			}
			return fmt.Errorf("environment value doesn't match. Expected: %s, Found: %s", testValue, value)
		}
	} else {
		if cleanupError := cleanup(clients); cleanupError != nil {
			t.Error(cleanupError)
		}
		return fmt.Errorf("%s not found in environment variables", testKey)
	}

	if err = cleanup(clients); err != nil {
		return err
	}
	return nil
}

func cleanupSecret(name string) func(clients *test.Clients) error {
	return func(clients *test.Clients) error {
		return clients.KubeClient.Kube.CoreV1().Secrets(test.ServingNamespace).Delete(name, nil)
	}
}

func cleanupConfigMap(name string) func(clients *test.Clients) error {
	return func(clients *test.Clients) error {
		return clients.KubeClient.Kube.CoreV1().ConfigMaps(test.ServingNamespace).Delete(name, nil)
	}
}
