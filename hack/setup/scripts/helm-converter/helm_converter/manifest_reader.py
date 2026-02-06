"""
Manifest Reader Module

Reads and parses Kubernetes manifests from the kustomize directory structure.
"""

import yaml
import subprocess
from pathlib import Path
from typing import Dict, List, Any


class ManifestReader:
    """Reads Kubernetes manifests based on mapping configuration"""

    def __init__(self, mapping_file: Path, repo_root: Path):
        self.mapping_file = mapping_file
        self.repo_root = repo_root

    def load_mapping(self) -> Dict[str, Any]:
        """Load the helm mapping YAML file with extends support"""
        with open(self.mapping_file, 'r') as f:
            mapping = yaml.safe_load(f)

        # Handle extends field
        if 'extends' in mapping:
            mapping = self._merge_extends(mapping)

        return mapping

    def _merge_extends(self, mapping: Dict[str, Any]) -> Dict[str, Any]:
        """
        Merge base mapping files specified in 'extends' field

        The extends field can be:
        - A single file path (string)
        - A list of file paths

        Files are merged in order, with later files overriding earlier ones.
        The current mapping overrides all extended mappings.
        """
        extends = mapping.pop('extends')  # Remove extends field from final mapping

        # Normalize to list
        if isinstance(extends, str):
            extends = [extends]

        # Load and merge all base mappings
        merged = {}
        for base_file in extends:
            # Resolve path relative to current mapping file
            base_path = self.mapping_file.parent / base_file

            if not base_path.exists():
                raise FileNotFoundError(f"Extended mapping file not found: {base_path}")

            with open(base_path, 'r') as f:
                base_mapping = yaml.safe_load(f)

            # Recursively handle extends in base file
            if 'extends' in base_mapping:
                # Temporarily change mapping_file context for recursive extends
                original_mapping_file = self.mapping_file
                self.mapping_file = base_path
                base_mapping = self._merge_extends(base_mapping)
                self.mapping_file = original_mapping_file

            # Deep merge base into merged
            merged = self._deep_merge(merged, base_mapping)

        # Finally merge current mapping on top
        merged = self._deep_merge(merged, mapping)

        return merged

    def _deep_merge(self, base: Dict[str, Any], override: Dict[str, Any]) -> Dict[str, Any]:
        """
        Deep merge two dictionaries, with override taking precedence

        Rules:
        - If both values are dicts, recursively merge
        - If both values are lists, concatenate (override first, then base)
        - Otherwise, override takes precedence
        """
        result = base.copy()

        for key, value in override.items():
            if key in result:
                # Both have the same key
                if isinstance(result[key], dict) and isinstance(value, dict):
                    # Both are dicts - recursively merge
                    result[key] = self._deep_merge(result[key], value)
                elif isinstance(result[key], list) and isinstance(value, list):
                    # Both are lists - concatenate (override first)
                    result[key] = value + result[key]
                else:
                    # Override wins
                    result[key] = value
            else:
                # New key from override
                result[key] = value

        return result

    def read_manifests(self, mapping: Dict[str, Any]) -> Dict[str, Any]:
        """
        Read all manifests referenced in the mapping file

        Returns:
            Dictionary with manifest data organized by component/resource type
        """
        manifests = {
            'common': {},
            'components': {},
            'deployments': {},
            'runtimes': [],
            'llmisvcConfigs': [],
            'crds': {}
        }

        # Read common resources (inferenceServiceConfig and certManager)
        # These are now top-level in the mapping, not under 'common'
        manifests['common'] = self._read_common_resources(mapping)

        # Read main component (kserve or llmisvc)
        chart_name = mapping['metadata']['name']
        if chart_name in mapping:
            component_config = mapping[chart_name]
            manifests['components'][chart_name] = self._read_component(component_config)

        # Read localmodel components if they exist
        if 'localmodel' in mapping:
            manifests['components']['localmodel'] = self._read_component(mapping['localmodel'])

        # Read localmodelnode component if exists
        if 'localmodelnode' in mapping:
            manifests['components']['localmodelnode'] = self._read_component(mapping['localmodelnode'])

        # Read cluster serving runtimes (support both 'clusterServingRuntimes' and 'runtimes' keys)
        if 'clusterServingRuntimes' in mapping:
            manifests['runtimes'] = self._read_runtimes(mapping['clusterServingRuntimes'])
        elif 'runtimes' in mapping:
            manifests['runtimes'] = self._read_runtimes(mapping['runtimes'])

        # Read llmisvcConfigs (similar to runtimes, not a component)
        if 'llmisvcConfigs' in mapping:
            manifests['llmisvcConfigs'] = self._read_llmisvc_configs(mapping['llmisvcConfigs'])

        # Read CRDs
        if 'crds' in mapping:
            manifests['crds'] = self._read_crds(mapping['crds'])

        return manifests

    def _read_common_resources(self, common_config: Dict[str, Any]) -> Dict[str, Any]:
        """Read common/base resources (inferenceServiceConfig and certManager)

        Note: common_config parameter name kept for compatibility, but it actually
        receives the full mapping dict which now has inferenceServiceConfig and certManager
        at the top level instead of under 'common'
        """
        resources = {}

        # Read inferenceservice-config ConfigMap from inferenceServiceConfig
        if 'inferenceServiceConfig' in common_config and 'configMap' in common_config['inferenceServiceConfig']:
            config_map_path = self.repo_root / common_config['inferenceServiceConfig']['configMap']['manifestPath']
            if config_map_path.exists():
                resources['inferenceservice-config'] = self._read_yaml_file(config_map_path)

        # Read cert-manager Issuer from certManager
        if 'certManager' in common_config and 'issuer' in common_config['certManager']:
            issuer_path = self.repo_root / common_config['certManager']['issuer']['manifestPath']
            if issuer_path.exists():
                resources['certManager-issuer'] = self._read_yaml_file(issuer_path)

        # Read ClusterStorageContainer from storageContainer
        if 'storageContainer' in common_config and 'clusterStorageContainer' in common_config['storageContainer']:
            # Use kustomize build to get the storage container with image transformations applied
            storage_container_dir = self.repo_root / 'config' / 'storagecontainers'
            if storage_container_dir.exists():
                storage_containers = self._read_kustomize_build(storage_container_dir)
                # Look for ClusterStorageContainer/default in the resources dictionary
                if 'ClusterStorageContainer/default' in storage_containers:
                    resources['storageContainer-default'] = storage_containers['ClusterStorageContainer/default']

        return resources

    def _read_component(self, component_config: Dict[str, Any]) -> Dict[str, Any]:
        """Read a component's manifests using kustomize build"""
        component_data = {
            'config': component_config,
            'manifests': {},
            'copyAsIs': component_config.get('copyAsIs', False)  # Flag for resources with Go templates
        }

        # Read component using kustomize build if manifestPath is specified
        if 'manifestPath' in component_config:
            component_path = self.repo_root / component_config['manifestPath']
            if component_path.exists():
                # Use kustomize build to get all resources
                component_data['manifests']['resources'] = self._read_kustomize_build(component_path)

        # Extract workloads from kustomize build results based on kind (generic approach)
        if 'resources' in component_data['manifests']:
            for config_key, config_value in component_config.items():
                # Check if this config entry has a 'kind' field (indicates a workload)
                if not isinstance(config_value, dict) or 'kind' not in config_value:
                    continue

                workload_kind = config_value['kind']
                workload_name = config_value.get('name')
                workload_namespace = config_value.get('namespace')

                # Find matching resource in kustomize build results
                for resource_key, resource in component_data['manifests']['resources'].items():
                    if (resource.get('kind') == workload_kind and
                            resource.get('metadata', {}).get('name') == workload_name and
                            (not workload_namespace or resource.get('metadata', {}).get('namespace') == workload_namespace)):
                        component_data['manifests'][config_key] = resource
                        break

        return component_data

    def _read_runtimes(self, runtimes_config: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Read ClusterServingRuntime manifests using kustomize build

        Uses kustomize build to get the final rendered manifests with all patches applied
        (e.g., image replacements from kustomization.yaml)
        """
        runtimes = []

        if 'runtimes' in runtimes_config:
            # Run kustomize build on config/runtimes to get all runtimes with patches applied
            runtimes_path = self.repo_root / 'config' / 'runtimes'
            if runtimes_path.exists():
                # Get all runtime manifests from kustomize build
                kustomize_runtimes = self._read_kustomize_build(runtimes_path)

                # Match each runtime config with its kustomize build result
                for runtime_config in runtimes_config['runtimes']:
                    runtime_name = runtime_config['name']
                    runtime_kind = runtime_config.get('kind', 'ClusterServingRuntime')

                    # Find the runtime in kustomize build results
                    resource_key = f"{runtime_kind}/{runtime_name}"
                    if resource_key in kustomize_runtimes:
                        # Extract enabledPath from enabled config for individual runtime conditional
                        config_with_enabled_path = runtime_config.copy()
                        if 'enabled' in runtime_config and 'valuePath' in runtime_config['enabled']:
                            config_with_enabled_path['enabledPath'] = runtime_config['enabled']['valuePath']

                        runtime_data = {
                            'config': config_with_enabled_path,
                            'manifest': kustomize_runtimes[resource_key]
                        }
                        runtimes.append(runtime_data)
                    else:
                        print(f"Warning: Runtime {runtime_name} not found in kustomize build results")

        return runtimes

    def _read_llmisvc_configs(self, llmisvc_configs_config: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Read LLMInferenceServiceConfig manifests directly from YAML files

        These resources contain Go templates and need to be copied as-is with original formatting
        """
        configs = []

        if 'manifestPath' in llmisvc_configs_config:
            manifest_path = self.repo_root / llmisvc_configs_config['manifestPath']
            if manifest_path.exists():
                # Read YAML files directly from the directory to preserve formatting
                for yaml_file in manifest_path.glob('*.yaml'):
                    if yaml_file.name == 'kustomization.yaml':
                        continue

                    # Read the original YAML text to preserve formatting
                    with open(yaml_file, 'r') as f:
                        original_yaml = f.read()

                    # Also parse it to get metadata
                    with open(yaml_file, 'r') as f:
                        manifest = yaml.safe_load(f)

                    if manifest:
                        config_data = {
                            'config': {
                                'name': manifest.get('metadata', {}).get('name', 'unnamed'),
                                'kind': manifest.get('kind', 'Unknown'),
                                'enabledPath': llmisvc_configs_config.get('enabled', {}).get('valuePath', 'llmisvcConfigs.enabled'),
                            },
                            'manifest': manifest,
                            'original_yaml': original_yaml,  # Store original YAML text
                            'original_filename': yaml_file.name,  # Store original filename
                            'copyAsIs': llmisvc_configs_config.get('copyAsIs', True)  # Default to True for these resources
                        }
                        configs.append(config_data)

        return configs

    def _read_crds(self, crds_config: Dict[str, Any]) -> Dict[str, Any]:
        """Read CRD manifests

        Note: CRD reading is not currently implemented in mapping files.
        CRDs are managed separately in kserve-crd chart.
        This method is kept for potential future use.
        """
        return {}

    def _read_yaml_file(self, file_path: Path) -> Any:
        """Read a single YAML file and return parsed content"""
        with open(file_path, 'r') as f:
            # Handle multi-document YAML
            docs = list(yaml.safe_load_all(f))
            if len(docs) == 1:
                return docs[0]
            return docs

    def _read_kustomize_build(self, component_path: Path) -> Dict[str, Any]:
        """
        Run kustomize build and parse the output into separate resources

        Returns:
            Dictionary with resources organized by kind and name
        """
        try:
            # Run kustomize build
            result = subprocess.run(
                ['kustomize', 'build', str(component_path)],
                capture_output=True,
                text=True,
                check=True,
                cwd=str(self.repo_root)
            )

            # Parse YAML documents
            documents = list(yaml.safe_load_all(result.stdout))

            # Organize resources by kind
            resources = {}
            for doc in documents:
                if not doc:
                    continue

                kind = doc.get('kind', 'Unknown')
                name = doc.get('metadata', {}).get('name', 'unnamed')

                # Create resource key: kind/name
                resource_key = f"{kind}/{name}"
                resources[resource_key] = doc

            return resources

        except subprocess.CalledProcessError as e:
            print(f"Warning: kustomize build failed for {component_path}: {e.stderr}")
            return {}
        except FileNotFoundError:
            print("Warning: kustomize command not found. Please install kustomize.")
            return {}
