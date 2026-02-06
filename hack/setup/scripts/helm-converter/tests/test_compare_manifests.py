"""
Tests for compare_manifests module - Mapper Mismatch Detection
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

from compare_manifests import (  # noqa: E402
    compare_configmap_data,
    compare_mapped_fields,
    compare_resources,
    normalize_resource,
)


class TestConfigMapMapperMismatch:
    """Test ConfigMap mapper mismatch detection for nested fields"""

    def test_configmap_nested_field_mapper_mismatch(self):
        """Test detection when Helm has nested fields not in Kustomize"""
        # Kustomize ConfigMap (source of truth)
        kustomize_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'localModel': '{"enabled": true, "jobNamespace": "default"}'
            }
        }

        # Helm ConfigMap (with extra nested fields from mapper)
        helm_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'localModel': '{"enabled": true, "jobNamespace": "default", "jobTTLSecondsAfterFinished": 100, "reconcilationFrequencyInSecs": 30}'
            }
        }

        # Mapper config
        mappers = {
            'test-mapper': {
                'inferenceServiceConfig': {
                    'configMap': {
                        'dataFields': {
                            'localModel': {
                                'jobTTLSecondsAfterFinished': {
                                    'path': 'data.localModel.jobTTLSecondsAfterFinished',
                                    'valuePath': 'config.localModel.jobTTL'
                                },
                                'reconcilationFrequencyInSecs': {
                                    'path': 'data.localModel.reconcilationFrequencyInSecs',
                                    'valuePath': 'config.localModel.reconcileFreq'
                                }
                            }
                        }
                    }
                }
            }
        }

        success, msg = compare_configmap_data(kustomize_cm, helm_cm, mappers)

        # Should fail due to mapper mismatch
        assert not success
        assert "MAPPER MISMATCH DETECTED" in msg
        assert "data.localModel.jobTTLSecondsAfterFinished" in msg
        assert "data.localModel.reconcilationFrequencyInSecs" in msg
        assert "test-mapper" in msg
        assert "Action needed:" in msg

    def test_configmap_top_level_key_mapper_mismatch(self):
        """Test detection when Helm has top-level keys not in Kustomize"""
        kustomize_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'router': '{"image": "router:v1"}'
            }
        }

        helm_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'router': '{"image": "router:v1"}',
                'service': '{"serviceClusterIPNone": false}'  # Extra key!
            }
        }

        mappers = {
            'test-mapper': {
                'inferenceServiceConfig': {
                    'configMap': {
                        'dataFields': {
                            'service': {
                                'serviceClusterIPNone': {
                                    'path': 'data.service.serviceClusterIPNone',
                                    'valuePath': 'config.service.clusterIPNone'
                                }
                            }
                        }
                    }
                }
            }
        }

        success, msg = compare_configmap_data(kustomize_cm, helm_cm, mappers)

        assert not success
        assert "MAPPER MISMATCH DETECTED" in msg
        assert "data.service" in msg
        assert "test-mapper" in msg

    def test_configmap_no_mapper_mismatch(self):
        """Test when ConfigMaps match (no mismatch)"""
        kustomize_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'router': '{"image": "router:v1", "memory": "256Mi"}'
            }
        }

        helm_cm = {
            'apiVersion': 'v1',
            'kind': 'ConfigMap',
            'metadata': {'name': 'test-config', 'namespace': 'test'},
            'data': {
                'router': '{"image": "router:v1", "memory": "256Mi"}'
            }
        }

        success, msg = compare_configmap_data(kustomize_cm, helm_cm, None)

        assert success
        assert "All data fields match" in msg


class TestDeploymentMapperMismatch:
    """Test Deployment/Service mapper mismatch detection"""

    def test_deployment_image_path_not_in_kustomize(self):
        """Test when mapper defines image path that doesn't exist in Kustomize"""
        kustomize_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': []  # No containers!
                    }
                }
            }
        }

        helm_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {'name': 'manager', 'image': 'test:v1'}
                        ]
                    }
                }
            }
        }

        mapper_config = {
            'image': {
                'repository': {
                    'path': 'spec.template.spec.containers[0].image+(:,0)',
                    'valuePath': 'controller.image'
                },
                'tag': {
                    'path': 'spec.template.spec.containers[0].image+(:,1)',
                    'valuePath': 'controller.tag'
                }
            }
        }

        success, msg = compare_mapped_fields(kustomize_deployment, helm_deployment, mapper_config)

        assert not success
        assert "MAPPER MISMATCH" in msg
        assert "image.repository" in msg
        assert "does NOT exist in Kustomize resource" in msg
        assert "Action needed:" in msg

    def test_deployment_resources_path_not_in_kustomize(self):
        """Test when mapper defines resources path that doesn't exist"""
        kustomize_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {'name': 'manager', 'image': 'test:v1'}
                            # No resources field!
                        ]
                    }
                }
            }
        }

        helm_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {
                                'name': 'manager',
                                'image': 'test:v1',
                                'resources': {'limits': {'cpu': '100m'}}
                            }
                        ]
                    }
                }
            }
        }

        mapper_config = {
            'resources': {
                'path': 'spec.template.spec.containers[0].resources',
                'valuePath': 'controller.resources'
            }
        }

        success, msg = compare_mapped_fields(kustomize_deployment, helm_deployment, mapper_config)

        assert not success
        assert "MAPPER MISMATCH" in msg
        assert "resources" in msg
        assert "does NOT exist in Kustomize resource" in msg

    def test_deployment_fields_match(self):
        """Test when mapped fields match between Kustomize and Helm"""
        kustomize_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {
                                'name': 'manager',
                                'image': 'kserve/controller:v1.0',
                                'resources': {'limits': {'cpu': '100m'}}
                            }
                        ]
                    }
                }
            }
        }

        helm_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy', 'namespace': 'test'},
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {
                                'name': 'manager',
                                'image': 'kserve/controller:v1.0',
                                'resources': {'limits': {'cpu': '100m'}}
                            }
                        ]
                    }
                }
            }
        }

        mapper_config = {
            'image': {
                'repository': {
                    'path': 'spec.template.spec.containers[0].image+(:,0)',
                    'valuePath': 'controller.image'
                },
                'tag': {
                    'path': 'spec.template.spec.containers[0].image+(:,1)',
                    'valuePath': 'controller.tag'
                }
            },
            'resources': {
                'path': 'spec.template.spec.containers[0].resources',
                'valuePath': 'controller.resources'
            }
        }

        success, msg = compare_mapped_fields(kustomize_deployment, helm_deployment, mapper_config)

        assert success
        assert "All mapped fields match" in msg


class TestMinorDifferences:
    """Test that Helm metadata differences are treated as info, not warnings"""

    def test_helm_labels_are_minor_differences(self):
        """Test that Helm standard labels are ignored in comparison"""
        kustomize_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {
                'name': 'test-deploy',
                'namespace': 'test',
                'labels': {
                    'app': 'test'
                }
            },
            'spec': {'replicas': 1}
        }

        helm_deployment = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {
                'name': 'test-deploy',
                'namespace': 'test',
                'labels': {
                    'app': 'test',
                    'helm.sh/chart': 'test-chart-1.0.0',
                    'app.kubernetes.io/managed-by': 'Helm',
                    'app.kubernetes.io/instance': 'test-release',
                    'app.kubernetes.io/name': 'test-deploy',
                    'app.kubernetes.io/version': 'v1.0'
                }
            },
            'spec': {'replicas': 1}
        }

        # Normalize both resources
        k_normalized = normalize_resource(kustomize_deployment)
        h_normalized = normalize_resource(helm_deployment)

        # After normalization, Helm labels should be removed
        assert k_normalized == h_normalized

    def test_compare_resources_shows_info_for_minor_differences(self):
        """Test that minor differences result in info message, not warning"""
        kustomize_docs = [
            {
                'apiVersion': 'apps/v1',
                'kind': 'Deployment',
                'metadata': {
                    'name': 'test-deploy',
                    'namespace': 'test',
                    'labels': {'app': 'test'}
                },
                'spec': {'replicas': 1}
            }
        ]

        helm_docs = [
            {
                'apiVersion': 'apps/v1',
                'kind': 'Deployment',
                'metadata': {
                    'name': 'test-deploy',
                    'namespace': 'test',
                    'labels': {
                        'app': 'test',
                        'helm.sh/chart': 'test-1.0.0',
                        'app.kubernetes.io/managed-by': 'Helm'
                    }
                },
                'spec': {'replicas': 1}
            }
        ]

        success, report = compare_resources(kustomize_docs, helm_docs, "test", exclude_crds=False)

        # Should succeed (minor differences are OK)
        assert success

        # Check report contains info message
        assert "ℹ️  PASS: Manifests are equivalent (Helm metadata added as expected)" in report
        assert "ℹ️  Resources with expected Helm metadata differences" in report
        assert "Helm standard labels/annotations added by _helpers.tpl" in report


class TestNormalization:
    """Test resource normalization functionality"""

    def test_normalize_removes_helm_labels(self):
        """Test that normalize_resource removes Helm-specific labels"""
        resource = {
            'apiVersion': 'v1',
            'kind': 'Service',
            'metadata': {
                'name': 'test-svc',
                'labels': {
                    'app': 'test',
                    'helm.sh/chart': 'test-1.0.0',
                    'app.kubernetes.io/managed-by': 'Helm',
                    'app.kubernetes.io/instance': 'release',
                    'app.kubernetes.io/name': 'test',
                    'app.kubernetes.io/version': 'v1.0'
                }
            }
        }

        normalized = normalize_resource(resource)

        # Only 'app' label should remain
        assert 'labels' in normalized['metadata']
        assert normalized['metadata']['labels'] == {'app': 'test'}
        assert 'helm.sh/chart' not in normalized['metadata']['labels']
        assert 'app.kubernetes.io/managed-by' not in normalized['metadata']['labels']

    def test_normalize_removes_helm_annotations(self):
        """Test that normalize_resource removes Helm-specific annotations"""
        resource = {
            'apiVersion': 'v1',
            'kind': 'Service',
            'metadata': {
                'name': 'test-svc',
                'annotations': {
                    'custom.annotation': 'value',
                    'meta.helm.sh/release-name': 'my-release',
                    'meta.helm.sh/release-namespace': 'default'
                }
            }
        }

        normalized = normalize_resource(resource)

        # Only custom annotation should remain
        assert 'annotations' in normalized['metadata']
        assert normalized['metadata']['annotations'] == {'custom.annotation': 'value'}
        assert 'meta.helm.sh/release-name' not in normalized['metadata']['annotations']
        assert 'meta.helm.sh/release-namespace' not in normalized['metadata']['annotations']

    def test_normalize_removes_empty_labels_annotations(self):
        """Test that empty labels/annotations dicts are removed"""
        resource = {
            'apiVersion': 'v1',
            'kind': 'Service',
            'metadata': {
                'name': 'test-svc',
                'labels': {
                    'helm.sh/chart': 'test-1.0.0'  # Will be removed, leaving empty dict
                }
            }
        }

        normalized = normalize_resource(resource)

        # Empty labels dict should be removed entirely
        assert 'labels' not in normalized['metadata']
