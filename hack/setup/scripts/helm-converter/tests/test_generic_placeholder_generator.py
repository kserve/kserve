"""
Tests for GenericPlaceholderGenerator module

Integration tests for generating Helm templates using placeholder substitution.
"""
import pytest
import tempfile
import shutil
from pathlib import Path

from helm_converter.generators.generic_placeholder_generator import GenericPlaceholderGenerator


class TestGenericPlaceholderGeneratorIntegration:
    """Integration tests for generic placeholder generator"""

    def setup_method(self):
        """Set up test fixtures"""
        self.temp_dir = Path(tempfile.mkdtemp())
        self.templates_dir = self.temp_dir / 'templates'
        self.templates_dir.mkdir(parents=True, exist_ok=True)

    def teardown_method(self):
        """Clean up test fixtures"""
        if self.temp_dir.exists():
            shutil.rmtree(self.temp_dir)

    def test_generate_clusterservingruntime_template(self):
        """Test generating ClusterServingRuntime template with image and resources"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'kserve-sklearnserver',
                    'enabledPath': 'runtimes.sklearn.enabled',
                    'image': {
                        'repository': {'valuePath': 'runtimes.sklearn.image.repository'},
                        'tag': {'valuePath': 'runtimes.sklearn.image.tag', 'fallback': 'kserve.version'}
                    },
                    'resources': {'valuePath': 'runtimes.sklearn.resources'}
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'ClusterServingRuntime',
                    'metadata': {'name': 'kserve-sklearnserver'},
                    'spec': {
                        'containers': [
                            {
                                'name': 'kserve-container',
                                'image': 'kserve/sklearnserver:v0.13.0',
                                'resources': {
                                    'limits': {'cpu': '1', 'memory': '2Gi'}
                                }
                            }
                        ]
                    }
                }
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

        # Verify template file was created
        output_file = self.templates_dir / 'runtimes' / 'clusterservingruntime_kserve-sklearnserver.yaml'
        assert output_file.exists()

        # Read and verify template content
        template_content = output_file.read_text()

        # Check conditional wrapping (dual conditional for runtimes)
        assert '{{- if .Values.runtimes.enabled }}' in template_content
        assert '{{- if .Values.runtimes.sklearn.enabled }}' in template_content

        # Check Helm labels placeholder
        assert '{{- include "test-chart.labels" . | nindent 4 }}' in template_content

        # Check image template with fallback
        assert '{{ .Values.runtimes.sklearn.image.repository }}:{{ .Values.runtimes.sklearn.image.tag | default .Values.kserve.version }}' in template_content

        # Check resources template
        assert '{{- toYaml .Values.runtimes.sklearn.resources | nindent 6 }}' in template_content

    def test_generate_clusterstoragecontainer_template(self):
        """Test generating ClusterStorageContainer template with container fields"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'cluster-storage-container',
                    'enabledPath': 'storageContainer.enabled',
                    'container': {
                        'name': {'valuePath': 'storageContainer.name'},
                        'image': {
                            'repository': {'valuePath': 'storageContainer.image.repository'},
                            'tag': {'valuePath': 'storageContainer.image.tag'}
                        },
                        'imagePullPolicy': {'valuePath': 'storageContainer.imagePullPolicy'},
                        'resources': {'valuePath': 'storageContainer.resources'}
                    }
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'ClusterStorageContainer',
                    'metadata': {'name': 'cluster-storage-container'},
                    'spec': {
                        'container': {
                            'name': 'storage-initializer',
                            'image': 'kserve/storage-initializer:v0.13.0',
                            'imagePullPolicy': 'IfNotPresent',
                            'resources': {
                                'limits': {'cpu': '1', 'memory': '1Gi'}
                            }
                        }
                    }
                }
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'common')

        # Verify template file was created
        output_file = self.templates_dir / 'common' / 'clusterstoragecontainer_cluster-storage-container.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check conditional wrapping for common resources
        assert '{{- if .Values.storageContainer.enabled }}' in template_content

        # Check container name template
        assert 'name: {{ .Values.storageContainer.name }}' in template_content

        # Check container image template
        assert 'image: {{ .Values.storageContainer.image.repository }}:{{ .Values.storageContainer.image.tag }}' in template_content

        # Check imagePullPolicy template
        assert 'imagePullPolicy: {{ .Values.storageContainer.imagePullPolicy }}' in template_content

        # Check container resources template
        assert '{{- toYaml .Values.storageContainer.resources | nindent 6 }}' in template_content

    def test_generate_template_with_copyasis(self):
        """Test generating template with copyAsIs flag (adds namespace)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-config',
                    'image': {
                        'repository': {'valuePath': 'test.image.repository'},
                        'tag': {'valuePath': 'test.image.tag'}
                    }
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'TestResource',
                    'metadata': {'name': 'test-config'},
                    'spec': {
                        'containers': [
                            {'name': 'test', 'image': 'test/image:v1.0.0'}
                        ]
                    }
                },
                'copyAsIs': True
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'llmisvcconfigs')

        output_file = self.templates_dir / 'llmisvcconfigs' / 'testresource_test-config.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check namespace was added for copyAsIs resources
        assert 'namespace: {{ .Release.Namespace }}' in template_content

    def test_generate_template_with_supported_uri_formats(self):
        """Test generating template with supportedUriFormats field"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-runtime',
                    'enabledPath': 'runtimes.test.enabled',
                    'supportedUriFormats': {'valuePath': 'runtimes.test.supportedUriFormats'}
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'ClusterServingRuntime',
                    'metadata': {'name': 'test-runtime'},
                    'spec': {
                        'containers': [
                            {'name': 'test', 'image': 'test/image:v1.0.0'}
                        ],
                        'supportedUriFormats': [
                            {'prefix': 's3://'},
                            {'prefix': 'gs://'}
                        ]
                    }
                }
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

        output_file = self.templates_dir / 'runtimes' / 'clusterservingruntime_test-runtime.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check supportedUriFormats template
        assert '{{- toYaml .Values.runtimes.test.supportedUriFormats | nindent 4 }}' in template_content

    def test_generate_template_with_workload_type(self):
        """Test generating template with workloadType field"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-runtime',
                    'enabledPath': 'runtimes.test.enabled',
                    'workloadType': {'valuePath': 'runtimes.test.workloadType'}
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'ClusterServingRuntime',
                    'metadata': {'name': 'test-runtime'},
                    'spec': {
                        'containers': [
                            {'name': 'test', 'image': 'test/image:v1.0.0'}
                        ],
                        'workloadType': 'Deployment'
                    }
                }
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

        output_file = self.templates_dir / 'runtimes' / 'clusterservingruntime_test-runtime.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check workloadType template
        assert 'workloadType: {{ .Values.runtimes.test.workloadType }}' in template_content

    def test_generate_template_with_original_filename(self):
        """Test using original filename when specified"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-config',
                    'image': {
                        'repository': {'valuePath': 'test.image.repository'},
                        'tag': {'valuePath': 'test.image.tag'}
                    }
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'TestResource',
                    'metadata': {'name': 'test-config'},
                    'spec': {
                        'containers': [
                            {'name': 'test', 'image': 'test/image:v1.0.0'}
                        ]
                    }
                },
                'original_filename': 'custom-name.yaml'
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

        # Verify original filename was used
        output_file = self.templates_dir / 'runtimes' / 'custom-name.yaml'
        assert output_file.exists()

    def test_generate_template_conditional_with_fallback(self):
        """Test conditional wrapping with fallback for common resources"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-resource',
                    'enabledPath': 'storageContainer.enabled',
                    'enabledFallback': 'kserve.enableStorageContainer',
                    'container': {
                        'image': {
                            'repository': {'valuePath': 'storageContainer.image.repository'},
                            'tag': {'valuePath': 'storageContainer.image.tag'}
                        }
                    }
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'ClusterStorageContainer',
                    'metadata': {'name': 'test-resource'},
                    'spec': {
                        'container': {
                            'name': 'test',
                            'image': 'test/image:v1.0.0'
                        }
                    }
                }
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'common')

        output_file = self.templates_dir / 'common' / 'clusterstoragecontainer_test-resource.yaml'
        assert output_file.exists()

        template_content = output_file.read_text()

        # Check conditional with fallback
        assert '{{- if .Values.storageContainer.enabled | default .Values.kserve.enableStorageContainer }}' in template_content

    def test_generate_empty_resource_list(self):
        """Test generating templates with empty resource list"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        # Should not raise error
        generator.generate_templates(self.templates_dir, [], 'runtimes')

        # Directory should not be created
        assert not (self.templates_dir / 'runtimes').exists()

    def test_generate_template_missing_config(self):
        """Test error handling when resource data is missing config"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'TestResource',
                    'metadata': {'name': 'test'}
                }
                # Missing 'config' field
            }
        ]

        with pytest.raises(ValueError, match="Resource data missing required field"):
            generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

    def test_generate_template_missing_manifest(self):
        """Test error handling when resource data is missing manifest"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {'name': 'test'}
                # Missing 'manifest' field
            }
        ]

        with pytest.raises(ValueError, match="Resource data missing required field"):
            generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

    def test_generate_template_missing_config_name(self):
        """Test error handling when config is missing name"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {},  # Missing 'name'
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'TestResource',
                    'metadata': {'name': 'test'}
                }
            }
        ]

        with pytest.raises(ValueError, match="Resource config missing required field 'name'"):
            generator.generate_templates(self.templates_dir, resource_list, 'runtimes')

    def test_generate_template_preserves_go_templates(self):
        """Test that Go templates in manifest are preserved (escaped)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        generator = GenericPlaceholderGenerator(mapping)

        resource_list = [
            {
                'config': {
                    'name': 'test-config'
                },
                'manifest': {
                    'apiVersion': 'serving.kserve.io/v1alpha1',
                    'kind': 'TestResource',
                    'metadata': {'name': 'test-config'},
                    'spec': {
                        'goTemplate': '{{ .Request }}',
                        'containers': [
                            {
                                'name': 'test',
                                'image': 'test/image:v1.0.0',
                                'env': [
                                    {'name': 'VAR', 'value': '{{ .Response }}'}
                                ]
                            }
                        ]
                    }
                },
                'copyAsIs': True
            }
        ]

        generator.generate_templates(self.templates_dir, resource_list, 'llmisvcconfigs')

        output_file = self.templates_dir / 'llmisvcconfigs' / 'testresource_test-config.yaml'
        template_content = output_file.read_text()

        # Go templates should be escaped with {{ "{{" }} and {{ "}}" }}
        assert '{{ "{{" }} .Request {{ "}}" }}' in template_content
        assert '{{ "{{" }} .Response {{ "}}" }}' in template_content
