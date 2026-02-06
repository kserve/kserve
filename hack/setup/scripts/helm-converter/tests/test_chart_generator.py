"""
Tests for ChartGenerator module
"""
import pytest
import yaml

from helm_converter.chart_generator import ChartGenerator


class TestChartGenerator:
    """Test ChartGenerator functionality"""

    def test_generate_chart_yaml(self, tmp_path):
        """Test Chart.yaml generation"""
        mapping = {
            'metadata': {
                'name': 'test-chart',
                'version': '1.0.0',
                'description': 'Test chart for unit testing',
                'appVersion': '1.0.0'
            }
        }

        generator = ChartGenerator(mapping, {}, tmp_path, tmp_path)
        generator.metadata_gen.generate_chart_yaml(tmp_path)

        chart_file = tmp_path / 'Chart.yaml'
        assert chart_file.exists()

        with open(chart_file, 'r') as f:
            chart_data = yaml.safe_load(f)

        assert chart_data['name'] == 'test-chart'
        assert chart_data['version'] == '1.0.0'
        assert chart_data['description'] == 'Test chart for unit testing'
        assert chart_data['appVersion'] == '1.0.0'

    def test_generate_helpers(self, tmp_path):
        """Test _helpers.tpl generation"""
        mapping = {
            'metadata': {
                'name': 'test-chart'
            }
        }

        # Create templates directory first
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator = ChartGenerator(mapping, {}, tmp_path, tmp_path)
        generator.metadata_gen.generate_helpers()

        helpers_file = templates_dir / '_helpers.tpl'
        assert helpers_file.exists()

        with open(helpers_file, 'r') as f:
            content = f.read()

        # Check that it contains expected helper definitions
        assert 'test-chart.name' in content
        assert 'test-chart.fullname' in content
        assert 'test-chart.labels' in content

    def test_generate_notes(self, tmp_path):
        """Test NOTES.txt generation"""
        mapping = {
            'metadata': {
                'name': 'test-chart'
            }
        }

        # Create templates directory first
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator = ChartGenerator(mapping, {}, tmp_path, tmp_path)
        generator.metadata_gen.generate_notes()

        notes_file = templates_dir / 'NOTES.txt'
        assert notes_file.exists()

        with open(notes_file, 'r') as f:
            content = f.read()

        # Check that it contains installation message (using template syntax)
        assert 'Thank you for installing {{ .Chart.Name }}' in content

    def test_namespace_filtering(self, tmp_path):
        """Test that Namespace resources are filtered out"""
        mapping = {
            'metadata': {
                'name': 'kserve'  # Use kserve so it's treated as main component
            }
        }

        # Create mock manifests with Namespace and ServiceAccount resources
        manifests = {
            'common': {},
            'components': {},
            'runtimes': [],
            'crds': {}
        }

        generator = ChartGenerator(mapping, manifests, tmp_path, tmp_path)

        # Test resources including Namespace
        resources = {
            'Namespace/kserve': {
                'apiVersion': 'v1',
                'kind': 'Namespace',
                'metadata': {'name': 'kserve'}
            },
            'ServiceAccount/kserve-sa': {
                'apiVersion': 'v1',
                'kind': 'ServiceAccount',
                'metadata': {'name': 'kserve-sa', 'namespace': 'kserve'}
            }
        }

        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()
        kserve_dir = templates_dir / 'kserve'
        kserve_dir.mkdir()

        # Process resources for main component (kserve)
        generator._generate_kustomize_resources(kserve_dir, 'kserve', resources)

        # Check that Namespace was not generated
        all_files = list(kserve_dir.glob('*.yaml'))
        namespace_files = [f for f in all_files if 'namespace' in f.name.lower()]
        assert len(namespace_files) == 0

        # Check that ServiceAccount was generated
        sa_files = [f for f in all_files if 'serviceaccount' in f.name.lower()]
        assert len(sa_files) == 1

    def test_issuer_template_generation(self, tmp_path):
        """Test cert-manager Issuer template generation"""
        mapping = {
            'metadata': {
                'name': 'test-chart'
            },
            'certManager': {
                'enabled': {
                    'valuePath': 'certManager.enabled',
                    'fallback': 'kserve.createSharedResources',
                    'defaultValue': True
                },
                'issuer': {
                    'kind': 'Issuer',
                    'name': 'selfsigned-issuer',
                    'manifestPath': 'config/certmanager/issuer.yaml'
                }
            }
        }

        manifests = {
            'common': {
                'certManager-issuer': {
                    'apiVersion': 'cert-manager.io/v1',
                    'kind': 'Issuer',
                    'metadata': {
                        'name': 'selfsigned-issuer',
                        'namespace': 'kserve'
                    },
                    'spec': {
                        'selfSigned': {}
                    }
                }
            },
            'components': {},
            'runtimes': [],
            'crds': {}
        }

        generator = ChartGenerator(mapping, manifests, tmp_path, tmp_path)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()
        common_dir = templates_dir / 'common'
        common_dir.mkdir()

        generator.common_gen._generate_issuer_template(common_dir, manifests['common']['certManager-issuer'])

        # File name follows {kind}_{name}.yaml pattern (issuer_selfsigned-issuer.yaml)
        issuer_file = common_dir / 'issuer_selfsigned-issuer.yaml'
        assert issuer_file.exists()

        with open(issuer_file, 'r') as f:
            content = f.read()

        # Check for certManager conditional with default fallback
        assert '{{- if .Values.certManager.enabled | default .Values.kserve.createSharedResources }}' in content
        # Check for namespace templating
        assert '{{ .Release.Namespace }}' in content
        # Check for labels include
        assert 'include "test-chart.labels"' in content

    def test_configmap_template_generation(self, tmp_path):
        """Test ConfigMap template generation with data fields"""
        mapping = {
            'metadata': {
                'name': 'test-chart'
            },
            'inferenceServiceConfig': {
                'enabled': {
                    'valuePath': 'inferenceServiceConfig.enabled',
                    'fallback': 'kserve.createSharedResources'
                },
                'configMap': {
                    'kind': 'ConfigMap',
                    'name': 'inferenceservice-config',
                    'manifestPath': 'config/configmap/inferenceservice.yaml',
                    'dataFields': [
                        {
                            'key': 'deploy',
                            'valuePath': 'inferenceServiceConfig.deploy',
                            'defaultValue': '{"defaultDeploymentMode": "Serverless"}'
                        }
                    ]
                }
            }
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'metadata': {
                        'name': 'inferenceservice-config',
                        'namespace': 'kserve'
                    },
                    'data': {
                        'deploy': '{"defaultDeploymentMode": "Serverless"}'
                    }
                }
            },
            'components': {},
            'runtimes': [],
            'crds': {}
        }

        generator = ChartGenerator(mapping, manifests, tmp_path, tmp_path)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()
        common_dir = templates_dir / 'common'
        common_dir.mkdir()

        generator.common_gen._generate_configmap_template(common_dir)

        # File name follows {kind}_{name}.yaml pattern (configmap_inferenceservice-config.yaml)
        configmap_file = common_dir / 'configmap_inferenceservice-config.yaml'
        assert configmap_file.exists()

        with open(configmap_file, 'r') as f:
            content = f.read()

        # Check for conditional with default fallback
        assert '{{- if .Values.inferenceServiceConfig.enabled | default .Values.kserve.createSharedResources }}' in content
        # Check for namespace templating
        assert '{{ .Release.Namespace }}' in content
        # Check for data field templating with literal block scalar and nindent
        assert 'deploy: |-' in content
        assert '{{- toJson .Values.inferenceServiceConfig.deploy | nindent 4 }}' in content


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
