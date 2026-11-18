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
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// ConfigRenderer names how LLMInferenceServiceConfig templates are rendered.
type ConfigRenderer string

const (
	// ConfigRendererJSON marshals the merged config to JSON and executes it as one
	// template.
	ConfigRendererJSON ConfigRenderer = constants.ConfigRendererJSON
	// ConfigRendererRecursive executes each string in the merged spec as its own
	// template.
	ConfigRendererRecursive ConfigRenderer = constants.ConfigRendererRecursive
)

// AnnotationConfigRenderer pins the renderer for one service, overriding the
// RecursiveConfigRender feature gate. A service whose configs only work under one
// renderer can stay on it while the cluster default changes.
const AnnotationConfigRenderer = constants.ConfigRendererAnnotationKey

// configRendererError reports a renderer annotation the webhook would have rejected,
// such as one set before the webhook checked it. Only a change to the annotation can
// fix it, so it is not retried.
type configRendererError struct {
	value string
}

func (e *configRendererError) Error() string {
	return fmt.Sprintf("unsupported %s annotation %q: supported values are %q and %q",
		AnnotationConfigRenderer, e.value, ConfigRendererRecursive, ConfigRendererJSON)
}

// configRendererFor returns the renderer for llmSvc. The service's annotation wins
// over the feature gate, and an empty annotation counts as unset. The annotation is
// normalized because the webhook accepts it case-insensitively and the API server
// stores it as written.
func configRendererFor(llmSvc *v1alpha2.LLMInferenceService, reconcilerConfig *Config) (ConfigRenderer, error) {
	if llmSvc != nil {
		if raw := strings.TrimSpace(llmSvc.GetAnnotations()[AnnotationConfigRenderer]); raw != "" {
			switch renderer := ConfigRenderer(strings.ToLower(raw)); renderer {
			case ConfigRendererRecursive, ConfigRendererJSON:
				return renderer, nil
			default:
				return "", &configRendererError{value: raw}
			}
		}
	}
	return defaultConfigRenderer(reconcilerConfig), nil
}

// defaultConfigRenderer returns the renderer the feature gate selects for services
// without the annotation.
func defaultConfigRenderer(reconcilerConfig *Config) ConfigRenderer {
	if reconcilerConfig != nil && reconcilerConfig.FeatureGates.RecursiveConfigRender {
		return ConfigRendererRecursive
	}
	return ConfigRendererJSON
}

// ValidateConfigTemplates renders config against a sample service for the
// LLMInferenceServiceConfig admission webhook.
//
// Any service can pick either renderer with AnnotationConfigRenderer, so a config
// that renders under one of them is admitted. When neither works, the error is the
// one from the cluster default.
func ValidateConfigTemplates(config *v1alpha2.LLMInferenceServiceConfig, reconcilerConfig *Config) error {
	sample := LLMInferenceServiceSample()

	renderer := defaultConfigRenderer(reconcilerConfig)
	_, err := renderConfig(renderer, sample, config, reconcilerConfig)
	if err == nil {
		return nil
	}

	other := ConfigRendererRecursive
	if renderer == ConfigRendererRecursive {
		other = ConfigRendererJSON
	}
	if _, otherErr := renderConfig(other, sample, config, reconcilerConfig); otherErr == nil {
		return nil
	}
	return err
}
