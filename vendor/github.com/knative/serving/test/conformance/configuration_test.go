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
	"reflect"
	"testing"

	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/test"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateConfigurationMetadata(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	names := test.ResourceNames{
		Config: test.ObjectNameForTest(t),
		Image:  pizzaPlanet1,
	}

	defer test.TearDown(clients, names)
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })

	t.Logf("Creating new configuration %s", names.Config)
	if _, err := test.CreateConfiguration(t, clients, names, &test.Options{}); err != nil {
		t.Fatalf("Failed to create configuration %s", names.Config)
	}

	t.Log("The Configuration will be updated with the name of the Revision once it is created")
	var err error
	names.Revision, err = waitForConfigurationLatestCreatedRevision(clients, names)
	if err != nil {
		t.Fatalf("Configuration %s was not updated with the new revision: %v", names.Config, err)
	}

	cfg := fetchConfiguration(names.Config, clients, t)

	t.Logf("Updating labels of Configuration %s", names.Config)
	newLabels := map[string]string{
		"labelX": "abc",
		"labelY": "def",
	}
	// Copy over new labels.
	if cfg.Labels == nil {
		cfg.Labels = newLabels
	} else {
		for k, v := range newLabels {
			cfg.Labels[k] = v
		}
	}
	cfg, err = clients.ServingClient.Configs.Update(cfg)
	if err != nil {
		t.Fatalf("Failed to update labels for Configuration %s: %v", names.Config, err)
	}

	if err = waitForConfigurationLabelsUpdate(clients, names, cfg.Labels); err != nil {
		t.Fatalf("The labels for Configuration %s were not updated: %v", names.Config, err)
	}

	cfg = fetchConfiguration(names.Config, clients, t)
	expected := names.Revision
	actual := cfg.Status.LatestCreatedRevisionName
	if expected != actual {
		t.Errorf("Did not expect a new Revision after updating labels for Configuration %s - expected Revision: %s, actual Revision: %s",
			names.Config, expected, actual)
	}

	t.Logf("Validating labels were not propagated to Revision %s", names.Revision)
	err = test.CheckRevisionState(clients.ServingClient, names.Revision, func(r *v1alpha1.Revision) (bool, error) {
		// Labels we placed on Configuration should _not_ appear on Revision.
		return checkNoKeysPresent(newLabels, r.Labels, t), nil
	})
	if err != nil {
		t.Errorf("The labels for Revision %s of Configuration %s should not have been updated: %v", names.Revision, names.Config, err)
	}

	t.Logf("Updating annotations of Configuration %s", names.Config)
	newAnnotations := map[string]string{
		"annotationA": "123",
		"annotationB": "456",
	}
	if cfg.Annotations == nil {
		cfg.Annotations = newAnnotations
	} else {
		// Copy over new annotations.
		for k, v := range newAnnotations {
			cfg.Annotations[k] = v
		}
	}
	cfg, err = clients.ServingClient.Configs.Update(cfg)
	if err != nil {
		t.Fatalf("Failed to update annotations for Configuration %s: %v", names.Config, err)
	}

	if err = waitForConfigurationAnnotationsUpdate(clients, names, cfg.Annotations); err != nil {
		t.Fatalf("The annotations for Configuration %s were not updated: %v", names.Config, err)
	}

	cfg = fetchConfiguration(names.Config, clients, t)
	actual = cfg.Status.LatestCreatedRevisionName
	if expected != actual {
		t.Errorf("Did not expect a new Revision after updating annotations for Configuration %s - expected Revision: %s, actual Revision: %s",
			names.Config, expected, actual)
	}

	t.Logf("Validating annotations were not propagated to Revision %s", names.Revision)
	err = test.CheckRevisionState(clients.ServingClient, names.Revision, func(r *v1alpha1.Revision) (bool, error) {
		// Annotations we placed on Configuration should _not_ appear on Revision.
		return checkNoKeysPresent(newAnnotations, r.Annotations, t), nil
	})
	if err != nil {
		t.Errorf("The annotations for Revision %s of Configuration %s should not have been updated: %v", names.Revision, names.Config, err)
	}
}

func fetchConfiguration(name string, clients *test.Clients, t *testing.T) *v1alpha1.Configuration {
	cfg, err := clients.ServingClient.Configs.Get(name, v1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get configuration %s: %v", name, err)
	}
	return cfg
}

func waitForConfigurationLatestCreatedRevision(clients *test.Clients, names test.ResourceNames) (string, error) {
	var revisionName string
	err := test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		if c.Status.LatestCreatedRevisionName != names.Revision {
			revisionName = c.Status.LatestCreatedRevisionName
			return true, nil
		}
		return false, nil
	}, "ConfigurationUpdatedWithRevision")
	return revisionName, err
}

func waitForConfigurationLabelsUpdate(clients *test.Clients, names test.ResourceNames, labels map[string]string) error {
	return test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		return reflect.DeepEqual(c.Labels, labels) && c.Generation == c.Status.ObservedGeneration, nil
	}, "ConfigurationMetadataUpdatedWithLabels")
}

func waitForConfigurationAnnotationsUpdate(clients *test.Clients, names test.ResourceNames, annotations map[string]string) error {
	return test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		return reflect.DeepEqual(c.Annotations, annotations) && c.Generation == c.Status.ObservedGeneration, nil
	}, "ConfigurationMetadataUpdatedWithAnnotations")
}

// checkNoKeysPresent returns true if _no_ keys from `expected`, are present in `actual`.
// checkNoKeysPresent will log the offending keys to t.Log.
func checkNoKeysPresent(expected map[string]string, actual map[string]string, t *testing.T) bool {
	t.Helper()
	present := []string{}
	for k := range expected {
		if _, ok := actual[k]; ok {
			present = append(present, k)
		}
	}
	if len(present) != 0 {
		t.Logf("Unexpected keys: %v", present)
	}
	return len(present) == 0
}
