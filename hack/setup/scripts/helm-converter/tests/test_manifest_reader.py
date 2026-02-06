"""
Tests for ManifestReader module
"""
import pytest
import yaml

from helm_converter.manifest_reader import ManifestReader


class TestManifestReader:
    """Test ManifestReader functionality"""

    def test_load_mapping_simple(self, tmp_path):
        """Test loading a simple mapping file"""
        # Create a simple mapping file
        mapping_data = {
            'metadata': {
                'name': 'test-chart',
                'version': '1.0.0'
            }
        }
        mapping_file = tmp_path / "test-mapping.yaml"
        with open(mapping_file, 'w') as f:
            yaml.dump(mapping_data, f)

        # Load the mapping
        reader = ManifestReader(mapping_file, tmp_path)
        result = reader.load_mapping()

        assert result['metadata']['name'] == 'test-chart'
        assert result['metadata']['version'] == '1.0.0'

    def test_extends_single_file(self, tmp_path):
        """Test extends with a single base file"""
        # Create base mapping
        base_data = {
            'common': {
                'enabled': {
                    'valuePath': 'common.enabled',
                    'defaultValue': True
                }
            }
        }
        base_file = tmp_path / "base.yaml"
        with open(base_file, 'w') as f:
            yaml.dump(base_data, f)

        # Create extending mapping
        extending_data = {
            'extends': 'base.yaml',
            'metadata': {
                'name': 'test-chart'
            }
        }
        extending_file = tmp_path / "extending.yaml"
        with open(extending_file, 'w') as f:
            yaml.dump(extending_data, f)

        # Load and verify
        reader = ManifestReader(extending_file, tmp_path)
        result = reader.load_mapping()

        assert 'common' in result
        assert result['common']['enabled']['defaultValue'] is True
        assert result['metadata']['name'] == 'test-chart'
        assert 'extends' not in result  # extends field should be removed

    def test_deep_merge_override(self, tmp_path):
        """Test that deep merge correctly overrides values"""
        # Create base mapping
        base_data = {
            'common': {
                'enabled': {
                    'defaultValue': True
                },
                'config': {
                    'setting1': 'base-value'
                }
            }
        }
        base_file = tmp_path / "base.yaml"
        with open(base_file, 'w') as f:
            yaml.dump(base_data, f)

        # Create extending mapping that overrides
        extending_data = {
            'extends': 'base.yaml',
            'common': {
                'enabled': {
                    'defaultValue': False  # Override
                }
            }
        }
        extending_file = tmp_path / "extending.yaml"
        with open(extending_file, 'w') as f:
            yaml.dump(extending_data, f)

        # Load and verify
        reader = ManifestReader(extending_file, tmp_path)
        result = reader.load_mapping()

        assert result['common']['enabled']['defaultValue'] is False  # Overridden
        assert result['common']['config']['setting1'] == 'base-value'  # Preserved

    def test_extends_file_not_found(self, tmp_path):
        """Test that extends raises error for missing file"""
        extending_data = {
            'extends': 'nonexistent.yaml',
            'metadata': {'name': 'test'}
        }
        extending_file = tmp_path / "extending.yaml"
        with open(extending_file, 'w') as f:
            yaml.dump(extending_data, f)

        reader = ManifestReader(extending_file, tmp_path)

        with pytest.raises(FileNotFoundError):
            reader.load_mapping()

    def test_extends_list(self, tmp_path):
        """Test extends with multiple base files"""
        # Create first base
        base1_data = {
            'common': {
                'setting1': 'value1'
            }
        }
        base1_file = tmp_path / "base1.yaml"
        with open(base1_file, 'w') as f:
            yaml.dump(base1_data, f)

        # Create second base
        base2_data = {
            'common': {
                'setting2': 'value2'
            }
        }
        base2_file = tmp_path / "base2.yaml"
        with open(base2_file, 'w') as f:
            yaml.dump(base2_data, f)

        # Create extending mapping with list
        extending_data = {
            'extends': ['base1.yaml', 'base2.yaml'],
            'metadata': {'name': 'test'}
        }
        extending_file = tmp_path / "extending.yaml"
        with open(extending_file, 'w') as f:
            yaml.dump(extending_data, f)

        # Load and verify
        reader = ManifestReader(extending_file, tmp_path)
        result = reader.load_mapping()

        assert result['common']['setting1'] == 'value1'
        assert result['common']['setting2'] == 'value2'
        assert result['metadata']['name'] == 'test'


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
