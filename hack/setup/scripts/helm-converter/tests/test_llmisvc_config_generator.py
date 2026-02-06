"""
Tests for LLMIsvcConfigGenerator module

Integration tests for generating LLMInferenceServiceConfig Helm templates.
"""
import pytest
import tempfile
import shutil
from pathlib import Path

from helm_converter.generators.llmisvc_config_generator import LLMIsvcConfigGenerator


class TestLLMIsvcConfigGeneratorIntegration:
    """Integration tests for LLMIsvc config generator"""

    def setup_method(self):
        """Set up test fixtures"""
        self.temp_dir = Path(tempfile.mkdtemp())
        self.templates_dir = self.temp_dir / 'templates'
        self.templates_dir.mkdir(parents=True, exist_ok=True)

    def teardown_method(self):
        """Clean up test fixtures"""
        if self.temp_dir.exists():
            shutil.rmtree(self.temp_dir)

    def test_generate_llmisvc_config_with_original_yaml(self):
        """Test generating LLMIsvc config with original YAML (preserves Go templates)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        # Original YAML with Go templates
        original_yaml = '''apiVersion: serving.kserve.io/v1alpha1
kind: LLMInferenceServiceConfig
metadata:
  name: kserve-config-granite-llm
  namespace: kserve
spec:
  llmAccelerator: gpu
  template:
    modelFormat:
      name: Granite
    params:
      requestTemplate: |
        {{- range .Inputs }}
        <|system|>{{ .System }}<|endoftext|>
        <|user|>{{ .Request }}<|endoftext|>
        <|assistant|>
        {{- end }}
      responseTemplate: |
        {{ .Response }}<|endoftext|>
'''

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {
                        'name': 'kserve-config-granite-llm'
                    },
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'kserve-config-granite-llm', 'namespace': 'kserve'}
                    },
                    'copyAsIs': True,
                    'original_yaml': original_yaml,
                    'original_filename': 'granite-llm.yaml'
                }
            ]
        }

        generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

        # Verify template file was created
        output_file = self.templates_dir / 'llmisvcconfigs' / 'granite-llm.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check conditional wrapping
        assert '{{- if .Values.llmisvcConfigs.enabled }}' in template_content

        # Check metadata
        assert 'apiVersion: serving.kserve.io/v1alpha1' in template_content
        assert 'kind: LLMInferenceServiceConfig' in template_content
        assert 'name: kserve-config-granite-llm' in template_content
        assert 'namespace: {{ .Release.Namespace }}' in template_content

        # Check Helm labels
        assert '{{- include "test-chart.labels" . | nindent 4 }}' in template_content

        # Check Go templates are preserved (escaped)
        assert '{{ "{{" }}- range .Inputs {{ "}}" }}' in template_content
        assert '{{ "{{" }} .System {{ "}}" }}' in template_content
        assert '{{ "{{" }} .Request {{ "}}" }}' in template_content
        assert '{{ "{{" }} .Response {{ "}}" }}' in template_content

    def test_generate_llmisvc_config_without_original_yaml(self):
        """Test generating LLMIsvc config without original YAML (fallback mode)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {
                        'name': 'kserve-config-test-llm'
                    },
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'kserve-config-test-llm'},
                        'spec': {
                            'llmAccelerator': 'gpu',
                            'template': {
                                'modelFormat': {
                                    'name': 'TestModel'
                                }
                            }
                        }
                    },
                    'copyAsIs': False
                }
            ]
        }

        generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

        # Verify template file was created with sanitized filename
        output_file = self.templates_dir / 'llmisvcconfigs' / 'test-llm.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check conditional wrapping
        assert '{{- if .Values.llmisvcConfigs.enabled }}' in template_content

        # Check metadata (no namespace for non-copyAsIs)
        assert 'name: kserve-config-test-llm' in template_content
        assert 'namespace: {{ .Release.Namespace }}' not in template_content

        # Check spec was included
        assert 'llmAccelerator: gpu' in template_content
        assert 'modelFormat:' in template_content
        assert 'name: TestModel' in template_content

    def test_generate_llmisvc_config_with_sanitized_filename(self):
        """Test filename sanitization for LLMIsvc configs"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {
                        'name': 'kserve-config-granite-llm'
                    },
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'kserve-config-granite-llm'}
                    },
                    'copyAsIs': False
                }
            ]
        }

        generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

        # Filename should be sanitized: remove 'kserve-config-' and 'kserve-' prefixes
        output_file = self.templates_dir / 'llmisvcconfigs' / 'granite-llm.yaml'
        assert output_file.exists()

    def test_generate_llmisvc_config_filename_sanitization_variations(self):
        """Test various filename sanitization patterns"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        test_cases = [
            ('kserve-config-test', 'test.yaml'),
            ('kserve-test', 'test.yaml'),
            ('test-config', 'test-config.yaml')
        ]

        for config_name, expected_filename in test_cases:
            manifests = {
                'llmisvcConfigs': [
                    {
                        'config': {'name': config_name},
                        'manifest': {
                            'apiVersion': 'serving.kserve.io/v1alpha1',
                            'kind': 'LLMInferenceServiceConfig',
                            'metadata': {'name': config_name}
                        },
                        'copyAsIs': False
                    }
                ]
            }

            # Clean up previous test
            configs_dir = self.templates_dir / 'llmisvcconfigs'
            if configs_dir.exists():
                shutil.rmtree(configs_dir)

            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

            output_file = configs_dir / expected_filename
            assert output_file.exists(), f"Expected {expected_filename} for config name {config_name}"

    def test_generate_llmisvc_config_with_original_filename(self):
        """Test using original filename when specified"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {
                        'name': 'kserve-config-test'
                    },
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'kserve-config-test'}
                    },
                    'copyAsIs': True,
                    'original_filename': 'custom-config.yaml'
                }
            ]
        }

        generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

        # Should use original filename
        output_file = self.templates_dir / 'llmisvcconfigs' / 'custom-config.yaml'
        assert output_file.exists()

    def test_generate_empty_llmisvc_configs(self):
        """Test generating templates with empty llmisvcConfigs"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {}

        # Should not raise error
        generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

        # Directory should not be created
        assert not (self.templates_dir / 'llmisvcconfigs').exists()

    def test_generate_llmisvc_config_missing_config(self):
        """Test error handling when config is missing"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'test'}
                    }
                    # Missing 'config' field
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc config data missing required field"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

    def test_generate_llmisvc_config_missing_manifest(self):
        """Test error handling when manifest is missing"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {'name': 'test'}
                    # Missing 'manifest' field
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc config data missing required field"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

    def test_generate_llmisvc_config_missing_config_name(self):
        """Test error handling when config name is missing"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {},  # Missing 'name'
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'test'}
                    }
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc config missing required field 'name'"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

    def test_generate_llmisvc_config_missing_manifest_apiversion(self):
        """Test error handling when manifest is missing apiVersion"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {'name': 'test'},
                    'manifest': {
                        # Missing 'apiVersion'
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {'name': 'test'}
                    },
                    'copyAsIs': True,
                    'original_yaml': 'spec:\n  test: value\n'
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc manifest missing required field"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

    def test_generate_llmisvc_config_missing_manifest_kind(self):
        """Test error handling when manifest is missing kind"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {'name': 'test'},
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        # Missing 'kind'
                        'metadata': {'name': 'test'}
                    },
                    'copyAsIs': False
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc manifest missing required field"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)

    def test_generate_llmisvc_config_missing_manifest_name(self):
        """Test error handling when manifest is missing metadata.name"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = LLMIsvcConfigGenerator(mapping)

        manifests = {
            'llmisvcConfigs': [
                {
                    'config': {'name': 'test'},
                    'manifest': {
                        'apiVersion': 'serving.kserve.io/v1alpha1',
                        'kind': 'LLMInferenceServiceConfig',
                        'metadata': {}  # Missing 'name'
                    },
                    'copyAsIs': False
                }
            ]
        }

        with pytest.raises(ValueError, match="LLMIsvc manifest missing required field"):
            generator.generate_llmisvc_configs_templates(self.templates_dir, manifests)
