"""
Component Builder Module

Builds component values (kserve, llmisvc, localmodel) including
controller manager and node agent configurations.
"""

from typing import Dict, Any, Optional
from .path_extractor import extract_from_manifest, process_field_with_priority
from .utils import get_container_field

# Special fields that require custom processing logic
SPECIAL_FIELDS = {'image', 'resources'}

# Special sections to skip during pod-level field extraction
SPECIAL_SECTIONS = {'containers', 'image', 'resources', 'kind', 'name', 'namespace', 'manifestPath'}


class ComponentBuilder:
    """Builds component values (controller + nodeAgent)"""

    def __init__(self, mapping: Dict[str, Any]):
        self.mapping = mapping

    def build_component_values(
        self,
        component_name: str,
        component_config: Dict[str, Any],
        manifests: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Build values for a component (kserve, llmisvc, or localmodel).

        Uses actual values from kustomize build result, not defaultValue from mapping.
        This ensures that helm template output matches kustomize build output.

        Args:
            component_name: Component name (e.g., 'kserve', 'llmisvc', 'localmodel')
            component_config: Component configuration from mapping
            manifests: Kubernetes manifests

        Returns:
            Component values dictionary
        """
        values = {}

        # Main components (kserve, llmisvc) don't need enabled flag - always installed
        # Only localmodel needs enabled flag
        chart_name = self.mapping['metadata']['name']
        is_main_component = component_name in [chart_name, 'kserve', 'llmisvc']

        if not is_main_component and 'enabled' in component_config:
            # localmodel is optional, default to False
            values['enabled'] = False

        # Version field handling removed - now use value: "" in mapping for version anchors

        # Get the component manifest
        # Generic workload configuration (Deployment, DaemonSet, etc.)
        component_data = manifests.get('components', {}).get(component_name)

        if component_data:
            for config_key, config_value in component_config.items():
                # Check if this is a workload config (has 'kind' field)
                if not isinstance(config_value, dict) or 'kind' not in config_value:
                    continue

                # Get the workload manifest
                workload_manifest = None
                if 'manifests' in component_data and config_key in component_data['manifests']:
                    workload_manifest = component_data['manifests'][config_key]

                # Build workload values using generic method
                workload_kind = config_value['kind']
                workload_values = self._build_workload_values(
                    component_name,
                    config_key,
                    config_value,
                    workload_manifest,
                    workload_kind
                )
                values.update(workload_values)

        return values

    def _build_workload_values(
        self,
        component_name: str,
        config_key: str,
        workload_config: Dict[str, Any],
        workload_manifest: Optional[Dict[str, Any]],
        workload_kind: str
    ) -> Dict[str, Any]:
        """Build workload values (generic for Deployment, DaemonSet, etc.).

        Unified method that replaces _build_controller_manager_values() and
        _build_node_agent_values().

        Args:
            component_name: Component name
            config_key: Key in component_config (e.g., 'controllerManager', 'nodeAgent')
            workload_config: Workload configuration from mapper
            workload_manifest: Workload manifest (Deployment, DaemonSet, etc.)
            workload_kind: Kind of workload ('Deployment', 'DaemonSet', etc.)

        Returns:
            Workload values dictionary
        """
        values = {}

        # Extract the base path from valuePath (e.g., "kserve.controller" from "kserve.controller.image")
        # Default to config_key if not specified
        workload_key = config_key

        # Try to extract workload_key from valuePath
        # Check both old structure (image at root) and new structure (image in containers)
        value_path = None
        if 'containers' in workload_config:
            # NEW structure: Get valuePath from first container's image
            first_container = next(iter(workload_config['containers'].values()))
            if 'image' in first_container and 'repository' in first_container['image']:
                value_path = first_container['image']['repository'].get('valuePath', '')
        elif 'image' in workload_config and 'repository' in workload_config['image']:
            # OLD structure: Get valuePath from root image
            value_path = workload_config['image']['repository'].get('valuePath', '')

        if value_path:
            parts = value_path.split('.')
            if len(parts) >= 2:
                # The second part is the workload key (e.g., 'controller', 'nodeAgent')
                workload_key = parts[1]

        values[workload_key] = {}

        # Determine workload type for _extract_image_values
        # 'Deployment' -> 'deployment', 'DaemonSet' -> 'daemonset'
        workload_type = workload_kind.lower()

        # Check if new containers section exists
        if 'containers' in workload_config and workload_manifest:
            # NEW structure: Use containers section
            containers_values = self._build_containers_values(
                workload_config['containers'],
                workload_manifest,
                component_name,
                workload_key,
                workload_kind
            )
            values[workload_key].update(containers_values)

            # Process pod-level fields (excludes containers section)
            pod_level_fields = self._extract_pod_level_fields(
                workload_config,
                workload_manifest,
                config_key
            )
            values[workload_key].update(pod_level_fields)
        else:
            # OLD structure: Backward compatibility
            # Process container fields at root level

            # Image configuration
            if 'image' in workload_config and workload_manifest:
                img_values = self._extract_image_values(
                    workload_config['image'],
                    workload_manifest,
                    component_name,
                    workload_key,
                    workload_type=workload_type
                )
                values[workload_key].update(img_values)

            # Resources configuration
            if 'resources' in workload_config and workload_manifest:
                res_config = workload_config['resources']
                if isinstance(res_config, dict) and 'path' in res_config:
                    try:
                        resources = extract_from_manifest(workload_manifest, res_config['path'])
                        if resources:
                            values[workload_key]['resources'] = resources
                    except (KeyError, IndexError, ValueError) as e:
                        print(f"Warning: Failed to extract resources using path '{res_config['path']}': {e}")
                        # Fallback to first container
                        actual_resources = get_container_field(workload_manifest, 'resources', default={})
                        if actual_resources:
                            values[workload_key]['resources'] = actual_resources
                else:
                    # No path field - fallback to first container (backward compatibility)
                    actual_resources = get_container_field(workload_manifest, 'resources', default={})
                    if actual_resources:
                        values[workload_key]['resources'] = actual_resources

            # Generic fields (nodeSelector, affinity, tolerations, etc.)
            # All fields except 'image' and 'resources' are processed generically
            generic_fields = self._extract_generic_fields(
                workload_config,
                workload_manifest,
                config_key
            )
            values[workload_key].update(generic_fields)

        return values

    def _extract_generic_fields(
        self,
        config: Dict[str, Any],
        manifest: Optional[Dict[str, Any]],
        manifest_type: str
    ) -> Dict[str, Any]:
        """Extract generic fields from manifest based on mapper configuration.

        This method provides generic field extraction for fields that follow
        the standard path-based extraction pattern. Special fields like 'image'
        and 'resources' that require complex processing are excluded.

        Supports nested mapper configurations for partial configurability.

        Args:
            config: Configuration from mapper (e.g., na_config, cm_config)
            manifest: Kubernetes manifest (Deployment or DaemonSet)
            manifest_type: 'controllerManager' or 'nodeAgent'

        Returns:
            Dictionary with extracted field values
        """
        result = {}

        if not manifest:
            return result

        # Process all fields generically
        for field_name, field_config in config.items():
            # Skip special fields - they have their own processing logic
            if field_name in SPECIAL_FIELDS:
                continue

            # Check if this is a leaf node with path or nested config
            if isinstance(field_config, dict) and 'path' in field_config:
                # Leaf node - extract value directly
                try:
                    value = extract_from_manifest(manifest, field_config['path'])
                    # Allow None check to include empty dict {} and empty list []
                    if value is not None:
                        result[field_name] = value
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract {field_name} for {manifest_type} "
                          f"using path '{field_config['path']}': {e}")

            elif isinstance(field_config, dict):
                # Nested config - check if it has sub-fields with path/valuePath
                nested_values = self._extract_nested_field_values(
                    field_config,
                    manifest,
                    manifest_type,
                    field_name
                )
                if nested_values:
                    result[field_name] = nested_values

        return result

    def _extract_nested_field_values(
        self,
        nested_config: Dict[str, Any],
        manifest: Dict[str, Any],
        manifest_type: str,
        parent_field: str
    ) -> Dict[str, Any]:
        """Extract values from nested mapper configuration.

        Recursively processes nested fields to extract values from manifest.

        Args:
            nested_config: Nested configuration (e.g., securityContext config)
            manifest: Kubernetes manifest
            manifest_type: Type of manifest for error messages
            parent_field: Parent field name for error messages

        Returns:
            Dictionary with extracted nested values

        Example:
            nested_config = {
                'runAsNonRoot': {
                    'path': 'spec.template.spec.securityContext.runAsNonRoot',
                    'valuePath': '...'
                }
            }

            Returns: {'runAsNonRoot': True}
        """
        result = {}

        for key, sub_config in nested_config.items():
            if isinstance(sub_config, dict) and 'path' in sub_config:
                # Leaf node with path
                try:
                    value = extract_from_manifest(manifest, sub_config['path'])
                    if value is not None:
                        result[key] = value
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract {parent_field}.{key} for {manifest_type} "
                          f"using path '{sub_config['path']}': {e}")

            elif isinstance(sub_config, dict):
                # Deeper nesting - recurse
                nested_values = self._extract_nested_field_values(
                    sub_config,
                    manifest,
                    manifest_type,
                    f"{parent_field}.{key}"
                )
                if nested_values:
                    result[key] = nested_values

        return result

    def _extract_image_values(
        self,
        img_config: Dict[str, Any],
        manifest: Dict[str, Any],
        component_name: str,
        key_name: str,
        workload_type: str = 'deployment'
    ) -> Dict[str, Any]:
        """Extract image values (repository, tag, pullPolicy) from manifest.

        Args:
            img_config: Image configuration from mapper
            manifest: Kubernetes manifest (Deployment or DaemonSet)
            component_name: Component name
            key_name: Key name for this image (e.g., 'controller', 'nodeAgent')
            workload_type: 'deployment' or 'daemonset'

        Returns:
            Dictionary with image values
        """
        values = {}

        # Extract repository
        repository = None
        if 'repository' in img_config:
            repo_config = img_config['repository']

            # Use priority-based extraction: value > path > fallback
            has_value, repository = process_field_with_priority(
                repo_config,
                manifest,
                extract_from_manifest
            )

            if not has_value:
                # No 'value' or 'path' field - fallback to first container (backward compatibility)
                actual_image = get_container_field(manifest, 'image', default='')
                repository = actual_image.rsplit(':', 1)[0] if ':' in actual_image else actual_image

        # Extract tag
        tag = None
        if 'tag' in img_config:
            tag_config = img_config['tag']

            # Use priority-based extraction: value > path > fallback
            has_value, tag = process_field_with_priority(
                tag_config,
                manifest,
                extract_from_manifest
            )

            if has_value:
                # Tag extracted from 'value' or 'path' field
                if tag and tag != '':
                    # Replace :latest with version for comparison consistency
                    chart_version = self.mapping['metadata'].get('appVersion', 'latest')
                    if chart_version != 'latest' and 'latest' in tag:
                        tag = tag.replace('latest', chart_version)
                # If tag is empty string from value: "", keep it as is
            else:
                # No 'value' or 'path' field - fallback to first container (backward compatibility)
                actual_image = get_container_field(manifest, 'image', default='')
                tag = actual_image.rsplit(':', 1)[1] if ':' in actual_image else 'latest'

        # Extract pullPolicy
        pull_policy = None
        if 'pullPolicy' in img_config:
            policy_config = img_config['pullPolicy']
            if 'path' in policy_config:
                # Use path field to extract pullPolicy
                try:
                    pull_policy = extract_from_manifest(manifest, policy_config['path'])
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract pullPolicy using path '{policy_config['path']}': {e}")
                    # Fallback to first container
                    pull_policy = get_container_field(manifest, 'imagePullPolicy', default='Always')
            else:
                # No path field - fallback to first container (backward compatibility)
                pull_policy = get_container_field(manifest, 'imagePullPolicy', default='Always')

        # Build values structure based on valuePath format
        repo_path = img_config.get('repository', {}).get('valuePath', '')

        # Determine if flat or nested structure
        # Flat: kserve.controller.image (2 dots) or llmisvc.controller.containers.manager.image (4 dots with 'containers')
        # Nested: kserve.controller.image.repository (3+ dots without 'containers')
        is_flat_structure = False
        if repo_path:
            dot_count = repo_path.count('.')
            if dot_count == 2:
                # Old structure: kserve.controller.image
                is_flat_structure = True
            elif 'containers' in repo_path and dot_count == 4:
                # New containers structure: llmisvc.controller.containers.manager.image
                is_flat_structure = True

        if is_flat_structure:
            # Flat structure: image value is string
            if repository is not None:
                values['image'] = repository
            if tag is not None:
                values['tag'] = tag
            if pull_policy is not None:
                values['imagePullPolicy'] = pull_policy
        else:
            # Nested structure: image value is dict
            values['image'] = {}
            if repository is not None:
                values['image']['repository'] = repository
            if tag is not None:
                values['image']['tag'] = tag
            if pull_policy is not None:
                values['image']['pullPolicy'] = pull_policy

        return values

    def _build_containers_values(
        self,
        containers_config: Dict[str, Any],
        workload_manifest: Dict[str, Any],
        component_name: str,
        workload_key: str,
        workload_kind: str
    ) -> Dict[str, Any]:
        """Build values for containers section.

        Args:
            containers_config: Mapper's containers section (e.g., {manager: {...}, sidecar: {...}})
            workload_manifest: Deployment/DaemonSet manifest
            component_name: Component name
            workload_key: Workload key (e.g., 'controller')
            workload_kind: 'Deployment' or 'DaemonSet'

        Returns:
            {'containers': {'manager': {...}, 'sidecar': {...}}}
        """
        result = {}

        # Get actual containers from manifest
        actual_containers = workload_manifest['spec']['template']['spec']['containers']

        # Build mapping: container_name -> container_manifest
        container_manifests = {c['name']: c for c in actual_containers}

        # Process each configured container
        for container_name, container_config in containers_config.items():
            if container_name not in container_manifests:
                print(f"Warning: Container '{container_name}' not found in manifest")
                continue

            container_manifest = container_manifests[container_name]

            # Extract values for this container
            container_values = self._build_single_container_values(
                container_config,
                container_manifest,
                component_name,
                workload_key,
                container_name,
                workload_kind.lower()
            )

            result[container_name] = container_values

        return {'containers': result}

    def _build_single_container_values(
        self,
        container_config: Dict[str, Any],
        container_manifest: Dict[str, Any],
        component_name: str,
        workload_key: str,
        container_name: str,
        workload_type: str
    ) -> Dict[str, Any]:
        """Build values for a single container.

        Similar to current workload value building, but for container level.
        Supports both absolute paths (spec.template.spec.containers[0].image) and
        container-relative paths (image, securityContext.runAsNonRoot).

        Args:
            container_config: Container configuration from mapper
            container_manifest: Container manifest dict
            component_name: Component name
            workload_key: Workload key (e.g., 'controller')
            container_name: Container name (e.g., 'manager')
            workload_type: 'deployment' or 'daemonset'

        Returns:
            Container values dictionary
        """
        values = {}

        # Wrap container manifest in workload structure for absolute path extraction
        wrapped_manifest = {
            'spec': {
                'template': {
                    'spec': {
                        'containers': [container_manifest]
                    }
                }
            }
        }

        # Image configuration
        if 'image' in container_config:
            img_config = container_config['image']
            # For container-relative paths, use _extract_image_values_container
            # which handles both path types
            img_values = self._extract_image_values_container(
                img_config,
                container_manifest,
                wrapped_manifest,
                component_name,
                f"{workload_key}.containers.{container_name}",
                workload_type=workload_type
            )
            values.update(img_values)

        # Resources configuration
        if 'resources' in container_config:
            res_config = container_config['resources']
            if isinstance(res_config, dict) and 'path' in res_config:
                # Determine which manifest to use based on path type
                manifest_to_use = self._select_manifest_for_path(
                    res_config, container_manifest, wrapped_manifest
                )
                try:
                    resources = extract_from_manifest(manifest_to_use, res_config['path'])
                    if resources:
                        values['resources'] = resources
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract resources for container '{container_name}' "
                          f"using path '{res_config['path']}': {e}")
                    # Fallback to direct access
                    actual_resources = container_manifest.get('resources', {})
                    if actual_resources:
                        values['resources'] = actual_resources
            else:
                # No path field - fallback to direct access
                actual_resources = container_manifest.get('resources', {})
                if actual_resources:
                    values['resources'] = actual_resources

        # Generic fields (securityContext, probes, etc.)
        # Pass both manifests - _extract_generic_fields will handle path detection
        generic_fields = self._extract_generic_fields_container(
            container_config,
            container_manifest,
            wrapped_manifest,
            f"container:{container_name}"
        )
        values.update(generic_fields)

        return values

    def _select_manifest_for_path(
        self,
        field_config: Dict[str, Any],
        container_manifest: Dict[str, Any],
        wrapped_manifest: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Select appropriate manifest based on path type.

        Supports both absolute paths (spec.template.spec.containers[0]...) and
        container-relative paths (image, securityContext.runAsNonRoot).

        Args:
            field_config: Field configuration with 'path'
            container_manifest: Container manifest dict
            wrapped_manifest: Container wrapped in workload structure

        Returns:
            Appropriate manifest to use for extraction
        """
        # Check if path is container-relative
        path = field_config.get('path', '')

        if not path:
            return wrapped_manifest

        # Absolute path starts with spec.template.spec.containers
        if path.startswith('spec.template.spec.containers'):
            return wrapped_manifest
        else:
            # Container-relative path
            return container_manifest

    def _extract_generic_fields_container(
        self,
        config: Dict[str, Any],
        container_manifest: Dict[str, Any],
        wrapped_manifest: Dict[str, Any],
        manifest_type: str
    ) -> Dict[str, Any]:
        """Extract generic fields from container with path type detection.

        Similar to _extract_generic_fields but supports container-relative paths.

        Args:
            config: Configuration from mapper
            container_manifest: Container manifest dict
            wrapped_manifest: Container wrapped in workload structure
            manifest_type: Type for error messages

        Returns:
            Dictionary with extracted field values
        """
        result = {}

        if not container_manifest:
            return result

        # Process all fields generically
        for field_name, field_config in config.items():
            # Skip special fields - they have their own processing logic
            if field_name in SPECIAL_FIELDS:
                continue

            # Check if this is a leaf node with path or nested config
            if isinstance(field_config, dict) and 'path' in field_config:
                # Leaf node - extract value directly
                # Select appropriate manifest based on path type
                manifest_to_use = self._select_manifest_for_path(
                    field_config, container_manifest, wrapped_manifest
                )
                try:
                    value = extract_from_manifest(manifest_to_use, field_config['path'])
                    # Allow None check to include empty dict {} and empty list []
                    if value is not None:
                        result[field_name] = value
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract {field_name} for {manifest_type} "
                          f"using path '{field_config['path']}': {e}")

            elif isinstance(field_config, dict):
                # Nested config - check if it has sub-fields with path/valuePath
                nested_values = self._extract_nested_field_values_container(
                    field_config,
                    container_manifest,
                    wrapped_manifest,
                    manifest_type,
                    field_name
                )
                if nested_values:
                    result[field_name] = nested_values

        return result

    def _extract_nested_field_values_container(
        self,
        nested_config: Dict[str, Any],
        container_manifest: Dict[str, Any],
        wrapped_manifest: Dict[str, Any],
        manifest_type: str,
        parent_field: str
    ) -> Dict[str, Any]:
        """Extract values from nested container config with path type detection.

        Similar to _extract_nested_field_values but supports container-relative paths.

        Args:
            nested_config: Nested configuration
            container_manifest: Container manifest dict
            wrapped_manifest: Container wrapped in workload structure
            manifest_type: Type of manifest for error messages
            parent_field: Parent field name for error messages

        Returns:
            Dictionary with extracted nested values
        """
        result = {}

        for key, sub_config in nested_config.items():
            if isinstance(sub_config, dict) and 'path' in sub_config:
                # Leaf node with path
                # Select appropriate manifest based on path type
                manifest_to_use = self._select_manifest_for_path(
                    sub_config, container_manifest, wrapped_manifest
                )
                try:
                    value = extract_from_manifest(manifest_to_use, sub_config['path'])
                    if value is not None:
                        result[key] = value
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract {parent_field}.{key} for {manifest_type} "
                          f"using path '{sub_config['path']}': {e}")

            elif isinstance(sub_config, dict):
                # Deeper nesting - recurse
                nested_values = self._extract_nested_field_values_container(
                    sub_config,
                    container_manifest,
                    wrapped_manifest,
                    manifest_type,
                    f"{parent_field}.{key}"
                )
                if nested_values:
                    result[key] = nested_values

        return result

    def _extract_image_values_container(
        self,
        img_config: Dict[str, Any],
        container_manifest: Dict[str, Any],
        wrapped_manifest: Dict[str, Any],
        component_name: str,
        key_name: str,
        workload_type: str = 'deployment'
    ) -> Dict[str, Any]:
        """Extract image values with support for container-relative paths.

        Similar to _extract_image_values but handles both absolute paths
        (spec.template.spec.containers[0].image) and container-relative paths (image).

        Args:
            img_config: Image configuration from mapper
            container_manifest: Container manifest dict
            wrapped_manifest: Container wrapped in workload structure
            component_name: Component name
            key_name: Key name for valuePath
            workload_type: 'deployment' or 'daemonset'

        Returns:
            Dictionary with image, tag, and imagePullPolicy values
        """
        values = {}

        # Extract repository
        repository = None
        if 'repository' in img_config:
            repo_config = img_config['repository']

            # Select appropriate manifest for path extraction
            manifest_to_use = self._select_manifest_for_path(
                repo_config, container_manifest, wrapped_manifest
            )

            # Use priority-based extraction: value > path > None
            has_value, repository = process_field_with_priority(
                repo_config,
                manifest_to_use,
                extract_from_manifest
            )

            if not has_value:
                # No 'value' or 'path' field - fallback to direct access
                actual_image = container_manifest.get('image', '')
                repository = actual_image.rsplit(':', 1)[0] if ':' in actual_image else actual_image

        # Extract tag
        tag = None
        if 'tag' in img_config:
            tag_config = img_config['tag']

            # Select appropriate manifest for path extraction
            manifest_to_use = self._select_manifest_for_path(
                tag_config, container_manifest, wrapped_manifest
            )

            # Use priority-based extraction: value > path > None
            has_value, tag = process_field_with_priority(
                tag_config,
                manifest_to_use,
                extract_from_manifest
            )

            if has_value:
                # Tag extracted from 'value' or 'path' field
                if tag and tag != '':
                    # Replace :latest with version for comparison consistency
                    chart_version = self.mapping['metadata'].get('appVersion', 'latest')
                    if chart_version != 'latest' and 'latest' in tag:
                        tag = tag.replace('latest', chart_version)
                # If tag is empty string from value: "", keep it as is
            else:
                # No 'value' or 'path' field - fallback to direct access
                actual_image = container_manifest.get('image', '')
                tag = actual_image.rsplit(':', 1)[1] if ':' in actual_image else 'latest'

        # Extract pullPolicy
        pull_policy = None
        if 'pullPolicy' in img_config:
            policy_config = img_config['pullPolicy']

            # Select appropriate manifest for path extraction
            manifest_to_use = self._select_manifest_for_path(
                policy_config, container_manifest, wrapped_manifest
            )

            # Use priority-based extraction: value > path > None
            has_value, pull_policy = process_field_with_priority(
                policy_config,
                manifest_to_use,
                extract_from_manifest
            )

            if not has_value:
                # No 'value' or 'path' field - fallback to direct access
                pull_policy = container_manifest.get('imagePullPolicy', 'Always')

        # Build values structure based on valuePath format
        repo_path = img_config.get('repository', {}).get('valuePath', '')

        # Determine if flat or nested structure
        # Flat: kserve.controller.image (2 dots) or llmisvc.controller.containers.manager.image (4 dots with 'containers')
        # Nested: kserve.controller.image.repository (3+ dots without 'containers')
        is_flat_structure = False
        if repo_path:
            dot_count = repo_path.count('.')
            if dot_count == 2:
                # Old structure: kserve.controller.image
                is_flat_structure = True
            elif 'containers' in repo_path and dot_count == 4:
                # New containers structure: llmisvc.controller.containers.manager.image
                is_flat_structure = True

        if is_flat_structure:
            # Flat structure: image value is string
            if repository is not None:
                values['image'] = repository
            if tag is not None:
                values['tag'] = tag
            if pull_policy is not None:
                values['imagePullPolicy'] = pull_policy
        else:
            # Nested structure: image value is dict
            values['image'] = {}
            if repository is not None:
                values['image']['repository'] = repository
            if tag is not None:
                values['image']['tag'] = tag
            if pull_policy is not None:
                values['image']['pullPolicy'] = pull_policy

        return values

    def _extract_pod_level_fields(
        self,
        config: Dict[str, Any],
        manifest: Optional[Dict[str, Any]],
        manifest_type: str
    ) -> Dict[str, Any]:
        """Extract pod-level fields from manifest.

        Excludes containers section and special fields.

        Args:
            config: Configuration from mapper
            manifest: Kubernetes manifest
            manifest_type: Type of manifest for error messages

        Returns:
            Dictionary with extracted pod-level field values
        """
        result = {}

        if not manifest:
            return result

        for field_name, field_config in config.items():
            if field_name in SPECIAL_SECTIONS:
                continue

            # Extract using generic logic
            if isinstance(field_config, dict) and 'path' in field_config:
                try:
                    value = extract_from_manifest(manifest, field_config['path'])
                    if value is not None:
                        result[field_name] = value
                except (KeyError, IndexError, ValueError) as e:
                    print(f"Warning: Failed to extract {field_name} for {manifest_type} "
                          f"using path '{field_config['path']}': {e}")

            elif isinstance(field_config, dict):
                # Nested config
                nested_values = self._extract_nested_field_values(
                    field_config,
                    manifest,
                    manifest_type,
                    field_name
                )
                if nested_values:
                    result[field_name] = nested_values

        return result
