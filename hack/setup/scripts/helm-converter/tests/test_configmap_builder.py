"""
Tests for ConfigMapBuilder module

Integration tests for building inferenceServiceConfig values from ConfigMap manifests.
"""
import json

from helm_converter.values_generator.configmap_builder import ConfigMapBuilder


class TestConfigMapBuilderIntegration:
    """Integration tests for ConfigMap value building"""

    def test_build_inference_service_config_basic(self):
        """Test building basic inferenceServiceConfig values"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {
                    'valuePath': 'inferenceServiceConfig.enabled',
                    'value': True
                },
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'path': 'data.deploy',
                            'valuePath': 'inferenceServiceConfig.deploy'
                        }
                    }
                }
            }
        }

        deploy_config = {
            'defaultDeploymentMode': 'Serverless'
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'metadata': {'name': 'inferenceservice-config'},
                    'data': {
                        'deploy': json.dumps(deploy_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Verify enabled flag
        assert result['enabled'] is True

        # Verify deploy configuration was extracted
        assert 'deploy' in result
        assert result['deploy']['defaultDeploymentMode'] == 'Serverless'

    def test_build_inference_service_config_nested_structure(self):
        """Test building inferenceServiceConfig with nested structure"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'agent': {
                            'image': {
                                'path': 'data.agent.image',
                                'valuePath': 'inferenceServiceConfig.agent.image'
                            },
                            'memoryRequest': {
                                'path': 'data.agent.memoryRequest',
                                'valuePath': 'inferenceServiceConfig.agent.memoryRequest'
                            }
                        }
                    }
                }
            }
        }

        agent_config = {
            'image': 'kserve/agent:v0.13.0',
            'memoryRequest': '100Mi'
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {
                        'agent': json.dumps(agent_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Verify nested agent configuration
        assert 'agent' in result
        assert result['agent']['image'] == 'kserve/agent:v0.13.0'
        assert result['agent']['memoryRequest'] == '100Mi'

    def test_build_inference_service_config_multiple_fields(self):
        """Test building inferenceServiceConfig with multiple top-level fields"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'path': 'data.deploy',
                            'valuePath': 'inferenceServiceConfig.deploy'
                        },
                        'logger': {
                            'path': 'data.logger',
                            'valuePath': 'inferenceServiceConfig.logger'
                        },
                        'agent': {
                            'path': 'data.agent',
                            'valuePath': 'inferenceServiceConfig.agent'
                        }
                    }
                }
            }
        }

        deploy_config = {'defaultDeploymentMode': 'Serverless'}
        logger_config = {'image': 'kserve/logger:v0.13.0'}
        agent_config = {'image': 'kserve/agent:v0.13.0'}

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {
                        'deploy': json.dumps(deploy_config),
                        'logger': json.dumps(logger_config),
                        'agent': json.dumps(agent_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Verify all three fields were extracted
        assert 'deploy' in result
        assert 'logger' in result
        assert 'agent' in result
        assert result['deploy']['defaultDeploymentMode'] == 'Serverless'
        assert result['logger']['image'] == 'kserve/logger:v0.13.0'
        assert result['agent']['image'] == 'kserve/agent:v0.13.0'

    def test_build_inference_service_config_with_value_field(self):
        """Test inferenceServiceConfig field with value override"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'defaultDeploymentMode': {
                                'path': 'data.deploy.defaultDeploymentMode',
                                'valuePath': 'inferenceServiceConfig.deploy.defaultDeploymentMode',
                                'value': 'RawDeployment'  # Override
                            }
                        }
                    }
                }
            }
        }

        deploy_config = {'defaultDeploymentMode': 'Serverless'}

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {
                        'deploy': json.dumps(deploy_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Value field should take precedence over path extraction
        assert result['deploy']['defaultDeploymentMode'] == 'RawDeployment'

    def test_build_inference_service_config_skip_metadata_fields(self):
        """Test that metadata fields (valuePath, description, etc.) are skipped"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'valuePath': 'inferenceServiceConfig.deploy',
                            'description': 'Deployment configuration',
                            'defaultDeploymentMode': {
                                'path': 'data.deploy.defaultDeploymentMode',
                                'valuePath': 'inferenceServiceConfig.deploy.defaultDeploymentMode'
                            }
                        }
                    }
                }
            }
        }

        deploy_config = {'defaultDeploymentMode': 'Serverless'}

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {
                        'deploy': json.dumps(deploy_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Metadata fields should not appear in result
        assert 'valuePath' not in result.get('deploy', {})
        assert 'description' not in result.get('deploy', {})
        assert 'defaultDeploymentMode' in result['deploy']

    def test_build_inference_service_config_missing_configmap(self):
        """Test behavior when ConfigMap manifest is not found"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'path': 'data.deploy',
                            'valuePath': 'inferenceServiceConfig.deploy'
                        }
                    }
                }
            }
        }

        # No ConfigMap in manifests
        manifests = {
            'common': {}
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Should only have enabled flag, no data fields
        assert 'enabled' in result
        assert 'deploy' not in result

    def test_build_inference_service_config_configmap_missing_data(self):
        """Test behavior when ConfigMap has no data field"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'deploy': {
                            'path': 'data.deploy',
                            'valuePath': 'inferenceServiceConfig.deploy'
                        }
                    }
                }
            }
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'metadata': {'name': 'inferenceservice-config'}
                    # No 'data' field
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Should only have enabled flag
        assert 'enabled' in result
        assert 'deploy' not in result

    def test_build_inference_service_config_deeply_nested(self):
        """Test building inferenceServiceConfig with deeply nested structure"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'batcher': {
                            'maxBatchSize': {
                                'path': 'data.batcher.maxBatchSize',
                                'valuePath': 'inferenceServiceConfig.batcher.maxBatchSize'
                            },
                            'timeoutSeconds': {
                                'path': 'data.batcher.timeoutSeconds',
                                'valuePath': 'inferenceServiceConfig.batcher.timeoutSeconds'
                            }
                        }
                    }
                }
            }
        }

        batcher_config = {
            'maxBatchSize': 32,
            'timeoutSeconds': 5
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {
                        'batcher': json.dumps(batcher_config)
                    }
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Verify deeply nested structure
        assert 'batcher' in result
        assert result['batcher']['maxBatchSize'] == 32
        assert result['batcher']['timeoutSeconds'] == 5

    def test_build_inference_service_config_enabled_fallback(self):
        """Test enabled flag fallback when not specified"""
        mapping = {
            'inferenceServiceConfig': {
                # No explicit enabled field
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {}
                }
            }
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {}
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Should not have enabled flag if not specified in mapping
        assert 'enabled' not in result

    def test_build_inference_service_config_simple_value(self):
        """Test ConfigMap field with simple (non-dict) value"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {
                        'simpleField': 'simple-value'  # Not a dict
                    }
                }
            }
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {}
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Simple value should be returned as-is
        assert result['simpleField'] == 'simple-value'

    def test_build_inference_service_config_empty_datafields(self):
        """Test ConfigMap with empty dataFields"""
        mapping = {
            'inferenceServiceConfig': {
                'enabled': {'value': True},
                'configMap': {
                    'name': 'inferenceservice-config',
                    'dataFields': {}
                }
            }
        }

        manifests = {
            'common': {
                'inferenceservice-config': {
                    'apiVersion': 'v1',
                    'kind': 'ConfigMap',
                    'data': {}
                }
            }
        }

        builder = ConfigMapBuilder()
        result = builder.build_inference_service_config_values(mapping, manifests)

        # Should only have enabled flag
        assert result == {'enabled': True}
