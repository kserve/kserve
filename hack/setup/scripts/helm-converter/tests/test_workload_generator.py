"""
Tests for WorkloadGenerator module - Deployment and DaemonSet generation
"""
import pytest

from helm_converter.generators.workload_generator import WorkloadGenerator


class TestWorkloadGenerator:
    """Test WorkloadGenerator functionality"""

    @pytest.fixture
    def mapping(self):
        """Common mapping configuration"""
        return {
            'metadata': {
                'name': 'test-chart'
            }
        }

    @pytest.fixture
    def deployment_manifest(self):
        """Sample Deployment manifest"""
        return {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {
                'name': 'test-controller',
                'namespace': 'test-ns',
                'labels': {
                    'app': 'test-controller'
                }
            },
            'spec': {
                'selector': {
                    'matchLabels': {
                        'app': 'test-controller'
                    }
                },
                'template': {
                    'metadata': {
                        'labels': {
                            'app': 'test-controller'
                        }
                    },
                    'spec': {
                        'serviceAccountName': 'test-sa',
                        'securityContext': {
                            'runAsNonRoot': True,
                            'seccompProfile': {
                                'type': 'RuntimeDefault'
                            }
                        },
                        'containers': [
                            {
                                'name': 'manager',
                                'image': 'test/controller:v1.0.0',
                                'imagePullPolicy': 'Always',
                                'resources': {
                                    'limits': {
                                        'cpu': '100m',
                                        'memory': '300Mi'
                                    },
                                    'requests': {
                                        'cpu': '100m',
                                        'memory': '200Mi'
                                    }
                                }
                            }
                        ],
                        'terminationGracePeriodSeconds': 10,
                        'volumes': [
                            {
                                'name': 'cert',
                                'secret': {
                                    'secretName': 'test-cert'
                                }
                            }
                        ]
                    }
                }
            }
        }

    @pytest.fixture
    def daemonset_manifest(self):
        """Sample DaemonSet manifest"""
        return {
            'apiVersion': 'apps/v1',
            'kind': 'DaemonSet',
            'metadata': {
                'name': 'test-agent',
                'namespace': 'test-ns',
                'labels': {
                    'app': 'test-agent'
                }
            },
            'spec': {
                'selector': {
                    'matchLabels': {
                        'app': 'test-agent'
                    }
                },
                'template': {
                    'metadata': {
                        'labels': {
                            'app': 'test-agent'
                        }
                    },
                    'spec': {
                        'nodeSelector': {
                            'disktype': 'ssd'
                        },
                        'affinity': {},
                        'tolerations': [],
                        'securityContext': {
                            'runAsNonRoot': True,
                            'seccompProfile': {
                                'type': 'RuntimeDefault'
                            }
                        },
                        'serviceAccountName': 'test-agent-sa',
                        'containers': [
                            {
                                'name': 'manager',
                                'image': 'test/agent:v1.0.0',
                                'imagePullPolicy': 'Always',
                                'resources': {
                                    'limits': {
                                        'cpu': '100m',
                                        'memory': '300Mi'
                                    },
                                    'requests': {
                                        'cpu': '100m',
                                        'memory': '200Mi'
                                    }
                                }
                            }
                        ],
                        'terminationGracePeriodSeconds': 10,
                        'volumes': [
                            {
                                'name': 'models',
                                'hostPath': {
                                    'path': '/models',
                                    'type': 'DirectoryOrCreate'
                                }
                            }
                        ]
                    }
                }
            }
        }

    def test_deployment_manifest_fields_preserved(self, tmp_path, mapping, deployment_manifest):
        """Test that all manifest fields are preserved in generated template"""
        # Component config WITHOUT mapper for securityContext
        # This tests that manifest fields without mapper are kept as static
        component_data = {
            'config': {
                'controllerManager': {
                    'image': {
                        'repository': {'valuePath': 'test.controller.image'},
                        'tag': {'valuePath': 'test.controller.tag'},
                        'pullPolicy': {'valuePath': 'test.controller.imagePullPolicy'}
                    },
                    'resources': {
                        'valuePath': 'test.controller.resources'
                    }
                    # NOTE: No mapper for securityContext - should be kept as static!
                }
            },
            'manifests': {
                'controllerManager': deployment_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_deployment(templates_dir, 'test', component_data, 'controllerManager')

        # Deployment filename now includes the workload name
        deployment_file = templates_dir / 'deployment_test-controller.yaml'
        assert deployment_file.exists()

        with open(deployment_file, 'r') as f:
            content = f.read()

        # Verify securityContext is present (static, not templated)
        assert 'securityContext:' in content
        assert 'runAsNonRoot: true' in content
        assert 'seccompProfile:' in content
        assert 'type: RuntimeDefault' in content

        # Verify serviceAccountName is present (static)
        assert 'serviceAccountName: test-sa' in content

        # Verify terminationGracePeriodSeconds is present (static)
        assert 'terminationGracePeriodSeconds: 10' in content

        # Verify volumes is present (static)
        assert 'volumes:' in content
        assert 'name: cert' in content

    def test_deployment_mapper_fields_templated(self, tmp_path, mapping, deployment_manifest):
        """Test that mapper-defined fields are templated with {{ .Values.xxx }}"""
        # Add a custom field to manifest and mapper
        deployment_manifest['spec']['template']['spec']['nodeSelector'] = {
            'disktype': 'ssd'
        }

        component_data = {
            'config': {
                'controllerManager': {
                    'image': {
                        'repository': {'valuePath': 'test.controller.image'},
                        'tag': {'valuePath': 'test.controller.tag'},
                        'pullPolicy': {'valuePath': 'test.controller.imagePullPolicy'}
                    },
                    'resources': {
                        'valuePath': 'test.controller.resources'
                    },
                    # Add nodeSelector to mapper
                    'nodeSelector': {
                        'valuePath': 'test.controller.nodeSelector'
                    }
                }
            },
            'manifests': {
                'controllerManager': deployment_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_deployment(templates_dir, 'test', component_data, 'controllerManager')

        deployment_file = templates_dir / 'deployment_test-controller.yaml'
        with open(deployment_file, 'r') as f:
            content = f.read()

        # Verify nodeSelector is templated (configurable)
        assert '{{- with .Values.test.controller.nodeSelector }}' in content
        assert 'nodeSelector:' in content
        assert '{{- toYaml . | nindent 8 }}' in content

    def test_daemonset_manifest_fields_preserved(self, tmp_path, mapping, daemonset_manifest):
        """Test that all DaemonSet manifest fields are preserved"""
        component_data = {
            'config': {
                'nodeAgent': {
                    'image': {
                        'repository': {'valuePath': 'test.agent.image'},
                        'tag': {'valuePath': 'test.agent.tag'},
                        'pullPolicy': {'valuePath': 'test.agent.imagePullPolicy'}
                    },
                    'resources': {
                        'valuePath': 'test.agent.resources'
                    },
                    # Add nodeSelector, affinity, tolerations to mapper
                    'nodeSelector': {
                        'valuePath': 'test.agent.nodeSelector'
                    },
                    'affinity': {
                        'valuePath': 'test.agent.affinity'
                    },
                    'tolerations': {
                        'valuePath': 'test.agent.tolerations'
                    }
                    # NOTE: No mapper for securityContext - should be kept as static!
                }
            },
            'manifests': {
                'nodeAgent': daemonset_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_daemonset(templates_dir, 'test', component_data, 'nodeAgent')

        # Find generated daemonset file
        daemonset_files = list(templates_dir.glob('daemonset_*.yaml'))
        assert len(daemonset_files) == 1

        with open(daemonset_files[0], 'r') as f:
            content = f.read()

        # Verify securityContext is present (static, not templated)
        assert 'securityContext:' in content
        assert 'runAsNonRoot: true' in content

        # Verify nodeSelector is templated (configurable)
        assert '{{- with .Values.test.agent.nodeSelector }}' in content

        # Verify affinity is templated with default dict (always rendered, preserves empty {})
        assert 'affinity: {{- toYaml (.Values.test.agent.affinity | default dict)' in content

        # Verify tolerations is templated with default list (always rendered, preserves empty [])
        assert 'tolerations: {{- toYaml (.Values.test.agent.tolerations | default list)' in content

    def test_new_manifest_field_auto_handled(self, tmp_path, mapping, deployment_manifest):
        """Test that new manifest fields are automatically handled without code changes"""
        # Add a new field to manifest that's not in mapper
        deployment_manifest['spec']['template']['spec']['hostNetwork'] = True
        deployment_manifest['spec']['template']['spec']['dnsPolicy'] = 'ClusterFirstWithHostNet'

        component_data = {
            'config': {
                'controllerManager': {
                    'image': {
                        'repository': {'valuePath': 'test.controller.image'},
                        'tag': {'valuePath': 'test.controller.tag'},
                        'pullPolicy': {'valuePath': 'test.controller.imagePullPolicy'}
                    },
                    'resources': {
                        'valuePath': 'test.controller.resources'
                    }
                    # NOTE: No mapper for hostNetwork or dnsPolicy
                }
            },
            'manifests': {
                'controllerManager': deployment_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_deployment(templates_dir, 'test', component_data, 'controllerManager')

        deployment_file = templates_dir / 'deployment_test-controller.yaml'
        with open(deployment_file, 'r') as f:
            content = f.read()

        # Verify new fields are automatically included as static
        assert 'hostNetwork: true' in content
        assert 'dnsPolicy: ClusterFirstWithHostNet' in content

    def test_nested_field_configuration(self, tmp_path, mapping):
        """Test nested field configuration (partial configurable)"""
        deployment_manifest = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {
                'name': 'test-deployment',  # This name is used for filename
                'namespace': 'test',
                'labels': {
                    'app': 'test-controller'
                }
            },
            'spec': {
                'selector': {
                    'matchLabels': {
                        'app': 'test-controller'
                    }
                },
                'template': {
                    'metadata': {
                        'labels': {
                            'app': 'test-controller'
                        }
                    },
                    'spec': {
                        'securityContext': {
                            'runAsNonRoot': True,
                            'seccompProfile': {
                                'type': 'RuntimeDefault'
                            }
                        },
                        'serviceAccountName': 'test-sa',
                        'containers': [{
                            'name': 'manager',
                            'image': 'test/controller:v1.0.0',
                            'imagePullPolicy': 'Always',
                            'resources': {
                                'limits': {
                                    'cpu': '100m',
                                    'memory': '128Mi'
                                }
                            }
                        }]
                    }
                }
            }
        }

        component_data = {
            'config': {
                'controllerManager': {
                    'image': {
                        'repository': {
                            'valuePath': 'test.controller.image'
                        },
                        'tag': {
                            'valuePath': 'test.controller.tag'
                        }
                    },
                    'resources': {
                        'valuePath': 'test.controller.resources'
                    },
                    # Nested securityContext - only runAsNonRoot is configurable
                    'securityContext': {
                        'runAsNonRoot': {
                            'path': 'spec.template.spec.securityContext.runAsNonRoot',
                            'valuePath': 'test.controller.securityContext.runAsNonRoot'
                        }
                        # seccompProfile not in mapper -> stays static
                    }
                }
            },
            'manifests': {
                'controllerManager': deployment_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_deployment(templates_dir, 'test', component_data, 'controllerManager')

        deployment_file = templates_dir / 'deployment_test-deployment.yaml'
        with open(deployment_file, 'r') as f:
            content = f.read()

        # Verify nested field is rendered correctly
        # runAsNonRoot should be configurable
        assert 'runAsNonRoot: {{ .Values.test.controller.securityContext.runAsNonRoot }}' in content

        # seccompProfile should be static
        assert 'seccompProfile:' in content
        assert 'type: RuntimeDefault' in content

        # Verify it's under securityContext parent
        assert 'securityContext:' in content

    def test_container_field_without_mapper_stays_static(self, tmp_path, mapping, deployment_manifest):
        """Test that container fields not in mapper remain static"""
        # Add command, env, ports to container
        deployment_manifest['spec']['template']['spec']['containers'][0]['command'] = ['/manager']
        deployment_manifest['spec']['template']['spec']['containers'][0]['env'] = [
            {
                'name': 'POD_NAMESPACE',
                'valueFrom': {
                    'fieldRef': {
                        'fieldPath': 'metadata.namespace'
                    }
                }
            }
        ]
        deployment_manifest['spec']['template']['spec']['containers'][0]['ports'] = [
            {
                'containerPort': 9443,
                'name': 'webhook-server',
                'protocol': 'TCP'
            }
        ]

        component_data = {
            'config': {
                'controllerManager': {
                    'image': {
                        'repository': {
                            'path': 'spec.template.spec.containers[0].image+(:,0)',
                            'valuePath': 'test.controller.image'
                        },
                        'tag': {
                            'path': 'spec.template.spec.containers[0].image+(:,1)',
                            'valuePath': 'test.controller.tag'
                        },
                        'pullPolicy': {
                            'path': 'spec.template.spec.containers[0].imagePullPolicy',
                            'valuePath': 'test.controller.imagePullPolicy'
                        }
                    },
                    'resources': {
                        'path': 'spec.template.spec.containers[0].resources',
                        'valuePath': 'test.controller.resources'
                    }
                    # NOTE: No mapper for command, env, ports, volumeMounts
                }
            },
            'manifests': {
                'controllerManager': deployment_manifest
            }
        }

        generator = WorkloadGenerator(mapping)
        templates_dir = tmp_path / 'templates'
        templates_dir.mkdir()

        generator.generate_deployment(templates_dir, 'test', component_data, 'controllerManager')

        deployment_file = templates_dir / 'deployment_test-controller.yaml'
        with open(deployment_file, 'r') as f:
            content = f.read()

        # Verify all container fields are static (no Helm templating)
        assert 'command:' in content
        assert '- /manager' in content  # Static command value

        assert 'env:' in content
        assert 'name: POD_NAMESPACE' in content  # Static env value

        assert 'ports:' in content
        assert 'containerPort: 9443' in content  # Static port value

        # Should NOT have any {{ .Values.xxx }} for these fields
        assert '.Values.test.controller.command' not in content
        assert '.Values.test.controller.env' not in content
        assert '.Values.test.controller.ports' not in content


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
