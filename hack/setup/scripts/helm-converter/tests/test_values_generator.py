"""
Tests for ValuesGenerator module
"""
import pytest

from helm_converter.values_gen import ValuesGenerator
from helm_converter.values_generator.path_extractor import extract_from_manifest


class TestValuesGenerator:
    """Test ValuesGenerator functionality"""

    def test_build_inference_service_config_values(self, tmp_path):
        """Test building inferenceServiceConfig values section"""
        mapping = {
            'metadata': {
                'name': 'test-chart',
                'version': '1.0.0'
            },
            'inferenceServiceConfig': {
                'enabled': {
                    'valuePath': 'inferenceServiceConfig.enabled'
                },
                'configMap': {
                    'manifestPath': 'config/configmap/inferenceservice.yaml',
                    'dataFields': {
                        'deploy': {
                            'defaultDeploymentMode': {
                                'path': 'data.deploy.defaultDeploymentMode',
                                'valuePath': 'inferenceServiceConfig.deploy.defaultDeploymentMode'
                            }
                        }
                    }
                }
            }
        }

        # Mock manifest with ConfigMap data
        manifests = {
            'common': {
                'ConfigMap/kserve/inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'metadata': {'name': 'inferenceservice-config'},
                    'data': {
                        'deploy': '{"defaultDeploymentMode": "Serverless"}',
                        '_example': 'example data'
                    }
                }
            }
        }

        generator = ValuesGenerator(mapping, manifests, tmp_path)
        values = generator._build_values()

        assert 'inferenceServiceConfig' in values
        assert values['inferenceServiceConfig']['enabled'] is True
        assert 'deploy' in values['inferenceServiceConfig']
        assert values['inferenceServiceConfig']['deploy']['defaultDeploymentMode'] == 'Serverless'
        assert '_example' not in values['inferenceServiceConfig']

    def test_build_certmanager_values(self, tmp_path):
        """Test building certManager values"""
        mapping = {
            'metadata': {'name': 'test-chart'},
            'certManager': {
                'enabled': {
                    'valuePath': 'certManager.enabled'
                }
            }
        }

        generator = ValuesGenerator(mapping, {}, tmp_path)
        values = generator._build_values()

        assert 'certManager' in values
        assert values['certManager']['enabled'] is True  # Default value

    def test_build_component_values(self, tmp_path):
        """Test building component values"""
        mapping = {
            'metadata': {'name': 'kserve'},
            'kserve': {
                'enabled': {
                    'valuePath': 'kserve.enabled'
                },
                'controllerManager': {
                    'kind': 'Deployment',
                    'name': 'kserve-controller-manager',
                    'image': {
                        'repository': {
                            'valuePath': 'kserve.controllerManager.image.repository'
                        },
                        'tag': {
                            'valuePath': 'kserve.controllerManager.image.tag'
                        }
                    },
                    'resources': {
                        'valuePath': 'kserve.controllerManager.resources'
                    }
                }
            }
        }

        # Mock manifest with Deployment
        manifests = {
            'components': {
                'kserve': {
                    'manifests': {
                        'controllerManager': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [{
                                            'image': 'kserve/kserve-controller:latest',
                                            'imagePullPolicy': 'Always',
                                            'resources': {
                                                'limits': {'cpu': '100m', 'memory': '300Mi'},
                                                'requests': {'cpu': '100m', 'memory': '200Mi'}
                                            }
                                        }]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        generator = ValuesGenerator(mapping, manifests, tmp_path)
        values = generator._build_values()

        assert 'kserve' in values
        assert 'controllerManager' in values['kserve']
        assert values['kserve']['controllerManager']['image']['repository'] == 'kserve/kserve-controller'
        assert values['kserve']['controllerManager']['image']['tag'] == 'latest'
        assert values['kserve']['controllerManager']['resources']['limits']['cpu'] == '100m'

    def test_build_localmodel_values(self, tmp_path):
        """Test building localmodel values with enabled flag"""
        mapping = {
            'metadata': {'name': 'kserve'},
            'localmodel': {
                'enabled': {
                    'valuePath': 'localmodel.enabled'
                },
                'controllerManager': {
                    'kind': 'Deployment',
                    'name': 'kserve-localmodel-controller',
                    'image': {
                        'repository': {
                            'valuePath': 'localmodel.controllerManager.image.repository'
                        },
                        'tag': {
                            'valuePath': 'localmodel.controllerManager.image.tag'
                        }
                    }
                }
            }
        }

        # Mock manifest with Deployment
        manifests = {
            'components': {
                'localmodel': {
                    'manifests': {
                        'controllerManager': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [{
                                            'image': 'kserve/kserve-localmodel-controller:latest'
                                        }]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        generator = ValuesGenerator(mapping, manifests, tmp_path)
        values = generator._build_values()

        assert 'localmodel' in values
        assert values['localmodel']['enabled'] is False
        assert values['localmodel']['controllerManager']['image']['repository'] == 'kserve/kserve-localmodel-controller'
        assert values['localmodel']['controllerManager']['image']['tag'] == 'latest'

    def test_component_values_use_path_field(self, tmp_path):
        """Test that mapper path field is actually used for value extraction"""
        mapping = {
            'metadata': {'name': 'kserve'},
            'kserve': {
                'controllerManager': {
                    'kind': 'Deployment',
                    'name': 'kserve-controller-manager',
                    'image': {
                        'repository': {
                            'path': 'spec.template.spec.containers[0].image+(:,0)',
                            'valuePath': 'kserve.controller.image'
                        },
                        'tag': {
                            'path': 'spec.template.spec.containers[0].image+(:,1)',
                            'valuePath': 'kserve.controller.tag'
                        },
                        'pullPolicy': {
                            'path': 'spec.template.spec.containers[0].imagePullPolicy',
                            'valuePath': 'kserve.controller.imagePullPolicy'
                        }
                    },
                    'resources': {
                        'path': 'spec.template.spec.containers[0].resources',
                        'valuePath': 'kserve.controller.resources'
                    }
                }
            }
        }

        manifests = {
            'components': {
                'kserve': {
                    'manifests': {
                        'controllerManager': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [{
                                            'image': 'custom-registry:5000/kserve/controller:v0.14.0',
                                            'imagePullPolicy': 'IfNotPresent',
                                            'resources': {
                                                'limits': {'cpu': '200m', 'memory': '400Mi'},
                                                'requests': {'cpu': '50m', 'memory': '100Mi'}
                                            }
                                        }]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        generator = ValuesGenerator(mapping, manifests, tmp_path)
        values = generator._build_values()

        # Verify split correctly handled registry:port
        assert values['kserve']['controller']['image'] == 'custom-registry:5000/kserve/controller'
        assert values['kserve']['controller']['tag'] == 'v0.14.0'
        assert values['kserve']['controller']['imagePullPolicy'] == 'IfNotPresent'
        assert values['kserve']['controller']['resources']['limits']['cpu'] == '200m'

    def test_backward_compatibility_no_path_field(self, tmp_path):
        """Test that mappers without path field still work (fallback to hardcoded)"""
        # This is the same as test_build_component_values but explicitly documents backward compatibility
        mapping = {
            'metadata': {'name': 'kserve'},
            'kserve': {
                'enabled': {
                    'valuePath': 'kserve.enabled'
                },
                'controllerManager': {
                    'kind': 'Deployment',
                    'name': 'kserve-controller-manager',
                    'image': {
                        'repository': {
                            # No path field - should fallback to hardcoded
                            'valuePath': 'kserve.controllerManager.image.repository'
                        },
                        'tag': {
                            'valuePath': 'kserve.controllerManager.image.tag'
                        }
                    },
                    'resources': {
                        'valuePath': 'kserve.controllerManager.resources'
                    }
                }
            }
        }

        # Standard manifest structure
        manifests = {
            'components': {
                'kserve': {
                    'manifests': {
                        'controllerManager': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [{
                                            'image': 'kserve/kserve-controller:latest',
                                            'imagePullPolicy': 'Always',
                                            'resources': {
                                                'limits': {'cpu': '100m', 'memory': '300Mi'},
                                                'requests': {'cpu': '100m', 'memory': '200Mi'}
                                            }
                                        }]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        generator = ValuesGenerator(mapping, manifests, tmp_path)
        values = generator._build_values()

        # Should still extract values using fallback logic
        assert values['kserve']['controllerManager']['image']['repository'] == 'kserve/kserve-controller'
        assert values['kserve']['controllerManager']['image']['tag'] == 'latest'
        assert values['kserve']['controllerManager']['resources']['limits']['cpu'] == '100m'


class TestPathParsing:
    """Test _get_value_from_path functionality"""

    def test_basic_path_navigation(self):
        """Test navigating manifest without split"""
        manifest = {
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {'image': 'kserve/controller:v1.0'}
                        ]
                    }
                }
            }
        }

        result = extract_from_manifest(
            manifest,
            'spec.template.spec.containers[0].image'
        )
        assert result == 'kserve/controller:v1.0'

    def test_image_split_repository(self):
        """Test split operation to extract repository"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'kserve/controller:v1.2.3'}
                ]
            }
        }

        repo = extract_from_manifest(
            manifest,
            'spec.containers[0].image+(:,0)'
        )
        assert repo == 'kserve/controller'

    def test_image_split_tag(self):
        """Test split operation to extract tag"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'kserve/controller:v1.2.3'}
                ]
            }
        }

        tag = extract_from_manifest(
            manifest,
            'spec.containers[0].image+(:,1)'
        )
        assert tag == 'v1.2.3'

    def test_image_with_registry_port(self):
        """Test handling registry:port/image:tag format"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'registry.io:5000/kserve/controller:v1.0'}
                ]
            }
        }

        # Repository should include registry:port
        repo = extract_from_manifest(
            manifest,
            'spec.containers[0].image+(:,0)'
        )
        assert repo == 'registry.io:5000/kserve/controller'

        # Tag should be just the version
        tag = extract_from_manifest(
            manifest,
            'spec.containers[0].image+(:,1)'
        )
        assert tag == 'v1.0'

    def test_image_without_tag(self):
        """Test image without tag (no colon)"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'kserve/controller'}
                ]
            }
        }

        # Repository works (whole string, no split happens)
        repo = extract_from_manifest(
            manifest,
            'spec.containers[0].image+(:,0)'
        )
        assert repo == 'kserve/controller'

        # Tag should raise IndexError (no second part after split)
        with pytest.raises(IndexError):
            extract_from_manifest(
                manifest,
                'spec.containers[0].image+(:,1)'
            )

    def test_path_not_found(self):
        """Test error when path doesn't exist"""
        manifest = {'spec': {}}

        with pytest.raises(KeyError):
            extract_from_manifest(
                manifest,
                'spec.nonexistent.field'
            )

    def test_array_index_out_of_bounds(self):
        """Test error when array index is out of bounds"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'kserve/controller:v1.0'}
                ]
            }
        }

        with pytest.raises(IndexError):
            extract_from_manifest(
                manifest,
                'spec.containers[5].image'  # Only index 0 exists
            )

    def test_invalid_split_format(self):
        """Test error with invalid split specification"""
        manifest = {
            'spec': {
                'containers': [
                    {'image': 'kserve/controller:v1.0'}
                ]
            }
        }

        # Missing parentheses
        with pytest.raises(ValueError):
            extract_from_manifest(
                manifest,
                'spec.containers[0].image+:,0'
            )


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
