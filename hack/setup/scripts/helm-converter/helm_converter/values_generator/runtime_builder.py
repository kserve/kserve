"""
Runtime Builder Module

Builds runtime values for ClusterServingRuntimes.
"""

from typing import Dict, Any
from .path_extractor import extract_from_manifest, process_field_with_priority
from .utils import get_runtime_container_field


class RuntimeBuilder:
    """Builds runtime values for ClusterServingRuntimes"""

    def __init__(self, mapping: Dict[str, Any]):
        self.mapping = mapping

    def build_runtime_values(
        self,
        runtime_key: str,
        manifests: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Build runtime values from specified key.

        Uses actual values from kustomize build result, not defaultValue from mapping.
        This ensures that helm template output matches kustomize build output.

        Args:
            runtime_key: Key in mapping ('clusterServingRuntimes' or 'runtimes')
            manifests: Kubernetes manifests

        Returns:
            Runtime values dictionary
        """
        runtimes_config = self.mapping[runtime_key]
        values = {}

        # Global enabled flag - extract using priority logic
        if 'enabled' in runtimes_config:
            has_value, enabled_value = process_field_with_priority(
                runtimes_config['enabled'],
                None,
                None
            )

            if has_value:
                values['enabled'] = enabled_value
            else:
                # Fallback: runtimes are typically enabled by default
                values['enabled'] = True

        # Individual runtime configurations
        for runtime_config in runtimes_config.get('runtimes', []):
            # Extract runtime key name from enabled.valuePath or enabledPath (backward compat)
            if 'enabled' in runtime_config and isinstance(runtime_config['enabled'], dict):
                enabled_value_path = runtime_config['enabled'].get('valuePath', '')
            else:
                # Backward compatibility: old format with enabledPath
                enabled_value_path = runtime_config.get('enabledPath', '')

            runtime_key_name = self._extract_runtime_key(enabled_value_path)
            runtime_name = runtime_config['name']

            # Find the corresponding manifest
            runtime_manifest = None
            for runtime_data in manifests.get('runtimes', []):
                if runtime_data['config']['name'] == runtime_name:
                    runtime_manifest = runtime_data['manifest']
                    break

            # Skip if manifest not found (shouldn't happen in production)
            if not runtime_manifest:
                continue

            values[runtime_key_name] = {}

            # Individual runtime enabled flag - extract using priority logic
            if 'enabled' in runtime_config:
                has_value, enabled_value = process_field_with_priority(
                    runtime_config['enabled'],
                    None,
                    None
                )

                if has_value:
                    values[runtime_key_name]['enabled'] = enabled_value
                else:
                    # Fallback: individual runtimes are enabled by default
                    values[runtime_key_name]['enabled'] = True
            else:
                # No enabled config - use default
                values[runtime_key_name]['enabled'] = True

            # Image configuration - use path field if available
            if 'image' in runtime_config:
                img_values = self._extract_runtime_image_values(
                    runtime_config['image'],
                    runtime_manifest,
                    runtime_key_name,
                    runtime_name
                )
                values[runtime_key_name]['image'] = img_values

            # Resources configuration - use path field if available
            if 'resources' in runtime_config:
                res_config = runtime_config['resources']
                if 'path' in res_config:
                    try:
                        resources = extract_from_manifest(runtime_manifest, res_config['path'])
                        if resources:
                            values[runtime_key_name]['resources'] = resources
                    except (KeyError, IndexError, ValueError) as e:
                        print(f"Warning: Failed to extract resources for {runtime_name}: {e}")
                        # Fallback to first container
                        actual_resources = get_runtime_container_field(runtime_manifest, 'resources', default={})
                        if actual_resources:
                            values[runtime_key_name]['resources'] = actual_resources
                else:
                    # No path field - fallback to first container (backward compatibility)
                    actual_resources = get_runtime_container_field(runtime_manifest, 'resources', default={})
                    if actual_resources:
                        values[runtime_key_name]['resources'] = actual_resources

        return values

    def _extract_runtime_image_values(
        self,
        img_config: Dict[str, Any],
        runtime_manifest: Dict[str, Any],
        runtime_key: str,
        runtime_name: str
    ) -> Dict[str, Any]:
        """Extract image values for a runtime.

        Args:
            img_config: Image configuration from mapper
            runtime_manifest: Runtime manifest
            runtime_key: Runtime key name (e.g., 'sklearn')
            runtime_name: Runtime full name

        Returns:
            Image values dictionary
        """
        img_values = {}

        # Extract repository using priority logic
        if 'repository' in img_config:
            repo_config = img_config['repository']
            has_value, repository = process_field_with_priority(
                repo_config,
                runtime_manifest,
                extract_from_manifest
            )

            if has_value:
                img_values['repository'] = repository
            else:
                # Fallback to first container (backward compatibility)
                actual_image = get_runtime_container_field(runtime_manifest, 'image', default='')
                repository = actual_image.rsplit(':', 1)[0] if ':' in actual_image else actual_image
                img_values['repository'] = repository

        # Extract tag using priority logic
        if 'tag' in img_config:
            tag_config = img_config['tag']
            has_value, tag = process_field_with_priority(
                tag_config,
                runtime_manifest,
                extract_from_manifest
            )

            if has_value:
                # Replace :latest with version for comparison consistency
                # Only apply if tag is not already empty string
                if tag and tag != '':
                    chart_version = self.mapping['metadata'].get('appVersion', 'latest')
                    if chart_version != 'latest' and 'latest' in tag:
                        tag = tag.replace('latest', chart_version)

                img_values['tag'] = tag
            else:
                # Fallback to first container (backward compatibility)
                actual_image = get_runtime_container_field(runtime_manifest, 'image', default='')
                tag = actual_image.rsplit(':', 1)[1] if ':' in actual_image else 'latest'
                img_values['tag'] = tag

        return img_values

    def _extract_runtime_key(self, enabled_path: str) -> str:
        """Extract runtime key from enabledPath.

        Args:
            enabled_path: Enabled path (e.g., "runtimes.sklearn.enabled")

        Returns:
            Runtime key (e.g., "sklearn")
        """
        parts = enabled_path.split('.')
        if len(parts) >= 2:
            return parts[1]
        return parts[0]
