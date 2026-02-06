"""
Tests for ComponentBuilder module

Integration tests for component value building from manifests.
"""
from helm_converter.values_generator.component_builder import ComponentBuilder


class TestComponentBuilderIntegration:
    """Integration tests for component value building"""

    def test_build_kserve_component_with_containers_structure(self):
        """Test building kserve component values with new containers structure"""
        mapping = {
            'metadata': {'name': 'kserve-resources', 'version': '1.0.0', 'appVersion': 'v0.13.0'}
        }
        builder = ComponentBuilder(mapping)

        component_config = {
            'controllerManager': {
                'kind': 'Deployment',
                'name': 'kserve-controller-manager',
                'containers': {
                    'manager': {
                        'image': {
                            'repository': {
                                'path': 'image+(:,0)',
                                'valuePath': 'kserve.controller.containers.manager.image'
                            },
                            'tag': {
                                'path': 'image+(:,1)',
                                'valuePath': 'kserve.controller.containers.manager.tag',
                                'value': '',
                                'fallback': 'kserve.version'
                            }
                        },
                        'resources': {
                            'path': 'resources',
                            'valuePath': 'kserve.controller.containers.manager.resources'
                        }
                    }
                }
            }
        }

        manifests = {
            'components': {
                'kserve': {
                    'manifests': {
                        'controllerManager': {
                            'apiVersion': 'apps/v1',
                            'kind': 'Deployment',
                            'metadata': {'name': 'kserve-controller-manager'},
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [
                                            {
                                                'name': 'manager',
                                                'image': 'kserve/kserve-controller:v0.13.0',
                                                'resources': {
                                                    'limits': {'cpu': '1', 'memory': '1Gi'},
                                                    'requests': {'cpu': '500m', 'memory': '512Mi'}
                                                }
                                            }
                                        ]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        result = builder.build_component_values('kserve', component_config, manifests)

        # Verify structure
        assert 'controller' in result
        assert 'containers' in result['controller']
        assert 'manager' in result['controller']['containers']

        # Verify image values
        manager_values = result['controller']['containers']['manager']
        assert manager_values['image'] == 'kserve/kserve-controller'
        assert manager_values['tag'] == ''  # Empty string from value: ""

        # Verify resources
        assert 'resources' in manager_values
        assert manager_values['resources']['limits']['cpu'] == '1'
        assert manager_values['resources']['requests']['memory'] == '512Mi'

    def test_build_localmodel_component_with_controller_and_agent(self):
        """Test building localmodel with both controller and nodeAgent"""
        mapping = {
            'metadata': {'name': 'kserve-localmodel-resources', 'version': '1.0.0'}
        }
        builder = ComponentBuilder(mapping)

        component_config = {
            'enabled': {
                'valuePath': 'localmodel.enabled'
            },
            'controllerManager': {
                'kind': 'Deployment',
                'name': 'kserve-localmodel-controller-manager',
                'containers': {
                    'manager': {
                        'image': {
                            'repository': {
                                'path': 'image+(:,0)',
                                'valuePath': 'localmodel.controller.containers.manager.image'
                            },
                            'tag': {
                                'path': 'image+(:,1)',
                                'valuePath': 'localmodel.controller.containers.manager.tag',
                                'value': ''
                            }
                        }
                    }
                }
            },
            'nodeAgent': {
                'kind': 'DaemonSet',
                'name': 'kserve-localmodelnode-agent',
                'containers': {
                    'manager': {
                        'image': {
                            'repository': {
                                'path': 'image+(:,0)',
                                'valuePath': 'localmodel.nodeAgent.containers.manager.image'
                            },
                            'tag': {
                                'path': 'image+(:,1)',
                                'valuePath': 'localmodel.nodeAgent.containers.manager.tag',
                                'value': ''
                            }
                        }
                    }
                }
            }
        }

        manifests = {
            'components': {
                'localmodel': {
                    'manifests': {
                        'controllerManager': {
                            'apiVersion': 'apps/v1',
                            'kind': 'Deployment',
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [
                                            {
                                                'name': 'manager',
                                                'image': 'kserve/localmodel-controller:v0.16.0'
                                            }
                                        ]
                                    }
                                }
                            }
                        },
                        'nodeAgent': {
                            'apiVersion': 'apps/v1',
                            'kind': 'DaemonSet',
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [
                                            {
                                                'name': 'manager',
                                                'image': 'kserve/localmodel-agent:v0.16.0'
                                            }
                                        ]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        result = builder.build_component_values('localmodel', component_config, manifests)

        # Verify enabled flag (localmodel is optional)
        assert result['enabled'] is False

        # Verify controller
        assert 'controller' in result
        assert result['controller']['containers']['manager']['image'] == 'kserve/localmodel-controller'
        assert result['controller']['containers']['manager']['tag'] == ''

        # Verify nodeAgent
        assert 'nodeAgent' in result
        assert result['nodeAgent']['containers']['manager']['image'] == 'kserve/localmodel-agent'
        assert result['nodeAgent']['containers']['manager']['tag'] == ''

    def test_build_component_with_imagepullpolicy(self):
        """Test building component with imagePullPolicy"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        builder = ComponentBuilder(mapping)

        component_config = {
            'controllerManager': {
                'kind': 'Deployment',
                'name': 'test-controller',
                'containers': {
                    'manager': {
                        'image': {
                            'repository': {
                                'path': 'image+(:,0)',
                                'valuePath': 'test.controller.containers.manager.image'
                            },
                            'tag': {
                                'path': 'image+(:,1)',
                                'valuePath': 'test.controller.containers.manager.tag'
                            },
                            'pullPolicy': {
                                'path': 'imagePullPolicy',
                                'valuePath': 'test.controller.containers.manager.imagePullPolicy'
                            }
                        }
                    }
                }
            }
        }

        manifests = {
            'components': {
                'test': {
                    'manifests': {
                        'controllerManager': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [
                                            {
                                                'name': 'manager',
                                                'image': 'test/image:v1.0.0',
                                                'imagePullPolicy': 'IfNotPresent'
                                            }
                                        ]
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        result = builder.build_component_values('test', component_config, manifests)

        manager_values = result['controller']['containers']['manager']
        assert manager_values['imagePullPolicy'] == 'IfNotPresent'

    def test_build_component_with_pod_level_fields(self):
        """Test building component with pod-level fields (nodeSelector, tolerations, affinity)"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        builder = ComponentBuilder(mapping)

        component_config = {
            'nodeAgent': {
                'kind': 'DaemonSet',
                'name': 'test-agent',
                'containers': {
                    'manager': {
                        'image': {
                            'repository': {
                                'path': 'image+(:,0)',
                                'valuePath': 'test.nodeAgent.containers.manager.image'
                            },
                            'tag': {
                                'path': 'image+(:,1)',
                                'valuePath': 'test.nodeAgent.containers.manager.tag'
                            }
                        }
                    }
                },
                'nodeSelector': {
                    'path': 'spec.template.spec.nodeSelector',
                    'valuePath': 'test.nodeAgent.nodeSelector'
                },
                'tolerations': {
                    'path': 'spec.template.spec.tolerations',
                    'valuePath': 'test.nodeAgent.tolerations'
                },
                'affinity': {
                    'path': 'spec.template.spec.affinity',
                    'valuePath': 'test.nodeAgent.affinity'
                }
            }
        }

        manifests = {
            'components': {
                'test': {
                    'manifests': {
                        'nodeAgent': {
                            'spec': {
                                'template': {
                                    'spec': {
                                        'containers': [
                                            {
                                                'name': 'manager',
                                                'image': 'test/agent:v1.0.0'
                                            }
                                        ],
                                        'nodeSelector': {
                                            'kubernetes.io/os': 'linux'
                                        },
                                        'tolerations': [
                                            {
                                                'key': 'node-role.kubernetes.io/control-plane',
                                                'operator': 'Exists',
                                                'effect': 'NoSchedule'
                                            }
                                        ],
                                        'affinity': {
                                            'nodeAffinity': {
                                                'requiredDuringSchedulingIgnoredDuringExecution': {
                                                    'nodeSelectorTerms': [
                                                        {
                                                            'matchExpressions': [
                                                                {
                                                                    'key': 'kubernetes.io/arch',
                                                                    'operator': 'In',
                                                                    'values': ['amd64', 'arm64']
                                                                }
                                                            ]
                                                        }
                                                    ]
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        result = builder.build_component_values('test', component_config, manifests)

        # Verify pod-level fields
        agent_values = result['nodeAgent']
        assert agent_values['nodeSelector']['kubernetes.io/os'] == 'linux'
        assert len(agent_values['tolerations']) == 1
        assert agent_values['tolerations'][0]['key'] == 'node-role.kubernetes.io/control-plane'
        assert 'affinity' in agent_values
        assert 'nodeAffinity' in agent_values['affinity']

    def test_build_component_empty_manifests(self):
        """Test building component when manifests are empty"""
        mapping = {
            'metadata': {'name': 'test-chart', 'version': '1.0.0'}
        }
        builder = ComponentBuilder(mapping)

        component_config = {
            'controllerManager': {
                'kind': 'Deployment',
                'name': 'test-controller'
            }
        }

        # Empty manifests
        manifests = {
            'components': {}
        }

        result = builder.build_component_values('test', component_config, manifests)

        # Should return empty dict or minimal structure
        assert isinstance(result, dict)
