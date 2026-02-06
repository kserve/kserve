"""
Tests for RuntimeBuilder module

Integration tests for ClusterServingRuntime value building from manifests.
"""
from helm_converter.values_generator.runtime_builder import RuntimeBuilder


class TestRuntimeBuilderIntegration:
    """Integration tests for runtime value building"""

    def test_build_single_runtime_values(self):
        """Test building values for a single runtime (sklearn)"""
        mapping = {
            'metadata': {'name': 'kserve-runtime-configs', 'version': '1.0.0', 'appVersion': 'v0.13.0'},
            'runtimes': {
                'enabled': {
                    'valuePath': 'runtimes.enabled',
                    'value': True
                },
                'runtimes': [
                    {
                        'name': 'kserve-sklearnserver',
                        'enabled': {
                            'valuePath': 'runtimes.sklearn.enabled',
                            'value': True
                        },
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.sklearn.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.sklearn.image.tag',
                                'value': '',
                                'fallback': 'kserve.version'
                            }
                        },
                        'resources': {
                            'path': 'spec.containers[0].resources',
                            'valuePath': 'runtimes.sklearn.resources'
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {
                        'name': 'kserve-sklearnserver'
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
                                        'limits': {'cpu': '1', 'memory': '2Gi'},
                                        'requests': {'cpu': '100m', 'memory': '256Mi'}
                                    }
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # Verify global enabled flag
        assert result['enabled'] is True

        # Verify sklearn runtime
        assert 'sklearn' in result
        assert result['sklearn']['enabled'] is True

        # Verify image
        assert 'image' in result['sklearn']
        assert result['sklearn']['image']['repository'] == 'kserve/sklearnserver'
        assert result['sklearn']['image']['tag'] == ''  # Empty from value: ""

        # Verify resources
        assert 'resources' in result['sklearn']
        assert result['sklearn']['resources']['limits']['cpu'] == '1'
        assert result['sklearn']['resources']['requests']['memory'] == '256Mi'

    def test_build_multiple_runtimes(self):
        """Test building values for multiple runtimes"""
        mapping = {
            'metadata': {'name': 'kserve-runtime-configs', 'version': '1.0.0'},
            'runtimes': {
                'enabled': {
                    'valuePath': 'runtimes.enabled',
                    'value': True
                },
                'runtimes': [
                    {
                        'name': 'kserve-sklearnserver',
                        'enabled': {
                            'valuePath': 'runtimes.sklearn.enabled',
                            'value': True
                        },
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.sklearn.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.sklearn.image.tag'
                            }
                        }
                    },
                    {
                        'name': 'kserve-xgbserver',
                        'enabled': {
                            'valuePath': 'runtimes.xgboost.enabled',
                            'value': True
                        },
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.xgboost.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.xgboost.image.tag'
                            }
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'kserve-sklearnserver'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'kserve/sklearnserver:v0.13.0'
                                }
                            ]
                        }
                    }
                },
                {
                    'config': {'name': 'kserve-xgbserver'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'kserve/xgbserver:v0.13.0'
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # Verify both runtimes exist
        assert 'sklearn' in result
        assert 'xgboost' in result

        # Verify sklearn
        assert result['sklearn']['image']['repository'] == 'kserve/sklearnserver'
        assert result['sklearn']['image']['tag'] == 'v0.13.0'

        # Verify xgboost
        assert result['xgboost']['image']['repository'] == 'kserve/xgbserver'
        assert result['xgboost']['image']['tag'] == 'v0.13.0'

    def test_runtime_with_empty_tag_and_fallback(self):
        """Test runtime with value: '' and fallback (commonly used pattern)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0', 'appVersion': 'v0.13.0'},
            'runtimes': {
                'enabled': {'value': True},
                'runtimes': [
                    {
                        'name': 'kserve-tensorflow',
                        'enabled': {'valuePath': 'runtimes.tensorflow.enabled', 'value': True},
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.tensorflow.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.tensorflow.image.tag',
                                'value': '',  # Empty value
                                'fallback': 'kserve.version'  # Fallback for template
                            }
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'kserve-tensorflow'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'kserve/tensorflow:v0.13.0'
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # Tag should be empty string (value: "")
        # Fallback is used in template generation, not in values.yaml
        assert result['tensorflow']['image']['tag'] == ''

    def test_runtime_without_resources(self):
        """Test runtime when resources config is not present"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'},
            'runtimes': {
                'enabled': {'value': True},
                'runtimes': [
                    {
                        'name': 'kserve-mlserver',
                        'enabled': {'valuePath': 'runtimes.mlserver.enabled', 'value': True},
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.mlserver.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.mlserver.image.tag'
                            }
                        }
                        # No resources config
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'kserve-mlserver'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'kserve/mlserver:v1.5.0',
                                    'resources': {
                                        'limits': {'memory': '1Gi'}
                                    }
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # Resources should not be in result when not configured in mapper
        assert 'resources' not in result['mlserver']

    def test_runtime_image_without_tag(self):
        """Test runtime image that has no tag (no colon)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'},
            'runtimes': {
                'enabled': {'value': True},
                'runtimes': [
                    {
                        'name': 'custom-runtime',
                        'enabled': {'valuePath': 'runtimes.custom.enabled', 'value': True},
                        'image': {
                            'repository': {
                                'path': 'spec.containers[0].image+(:,0)',
                                'valuePath': 'runtimes.custom.image.repository'
                            },
                            'tag': {
                                'path': 'spec.containers[0].image+(:,1)',
                                'valuePath': 'runtimes.custom.image.tag'
                            }
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'custom-runtime'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'custom/runtime'  # No tag
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # When path extraction fails (no colon in image), returns None
        # Template will use fallback if configured (| default .Values.xxx)
        assert result['custom']['image']['repository'] == 'custom/runtime'
        assert result['custom']['image']['tag'] is None  # Path extraction failed

    def test_extract_runtime_key_from_path(self):
        """Test _extract_runtime_key helper method"""
        mapping = {'metadata': {'name': 'test', 'version': '1.0.0'}}
        builder = RuntimeBuilder(mapping)

        # Standard format: runtimes.{key}.enabled
        assert builder._extract_runtime_key('runtimes.sklearn.enabled') == 'sklearn'
        assert builder._extract_runtime_key('runtimes.xgboost.enabled') == 'xgboost'
        assert builder._extract_runtime_key('runtimes.tensorflow.enabled') == 'tensorflow'

        # Edge cases
        assert builder._extract_runtime_key('sklearn') == 'sklearn'
        assert builder._extract_runtime_key('runtimes.mlserver') == 'mlserver'

    def test_runtime_missing_manifest(self):
        """Test behavior when runtime manifest is not found"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'},
            'runtimes': {
                'enabled': {'value': True},
                'runtimes': [
                    {
                        'name': 'kserve-sklearnserver',
                        'enabled': {'valuePath': 'runtimes.sklearn.enabled', 'value': True},
                        'image': {
                            'repository': {'valuePath': 'runtimes.sklearn.image.repository'},
                            'tag': {'valuePath': 'runtimes.sklearn.image.tag'}
                        }
                    },
                    {
                        'name': 'kserve-missing-runtime',
                        'enabled': {'valuePath': 'runtimes.missing.enabled', 'value': True},
                        'image': {
                            'repository': {'valuePath': 'runtimes.missing.image.repository'},
                            'tag': {'valuePath': 'runtimes.missing.image.tag'}
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        # Only sklearn manifest provided, missing-runtime not provided
        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'kserve-sklearnserver'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {'name': 'kserve-container', 'image': 'kserve/sklearnserver:v0.13.0'}
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # sklearn should exist
        assert 'sklearn' in result

        # missing should be skipped (not added to result)
        assert 'missing' not in result

    def test_runtime_with_path_extraction_fallback(self):
        """Test runtime with path extraction that falls back to direct access"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'},
            'runtimes': {
                'enabled': {'value': True},
                'runtimes': [
                    {
                        'name': 'kserve-paddle',
                        'enabled': {'valuePath': 'runtimes.paddle.enabled', 'value': True},
                        'image': {
                            'repository': {
                                'path': 'invalid.path.that.does.not.exist',
                                'valuePath': 'runtimes.paddle.image.repository'
                            },
                            'tag': {
                                'path': 'invalid.path.that.does.not.exist',
                                'valuePath': 'runtimes.paddle.image.tag'
                            }
                        }
                    }
                ]
            }
        }

        builder = RuntimeBuilder(mapping)

        manifests = {
            'runtimes': [
                {
                    'config': {'name': 'kserve-paddle'},
                    'manifest': {
                        'spec': {
                            'containers': [
                                {
                                    'name': 'kserve-container',
                                    'image': 'kserve/paddleserver:v0.13.0'
                                }
                            ]
                        }
                    }
                }
            ]
        }

        result = builder.build_runtime_values('runtimes', manifests)

        # When path extraction fails, returns None (not fallback to container)
        # Fallback happens in template generation, not values extraction
        assert result['paddle']['image']['repository'] is None
        assert result['paddle']['image']['tag'] is None
