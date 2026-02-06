import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))
from convert import _read_kserve_version  # noqa: E402


class TestReadKserveVersion:
    """Test kserve-deps.env parsing"""

    def test_read_valid_version(self, tmp_path):
        """Test reading valid KSERVE_VERSION"""
        deps_file = tmp_path / 'kserve-deps.env'
        deps_file.write_text('''
# Development tools
GOLANGCI_LINT_VERSION=v1.64.8

# KServe
KSERVE_VERSION=v0.16.0

# Other
ISTIO_VERSION=1.27.1
''')

        version = _read_kserve_version(tmp_path)
        assert version == 'v0.16.0'

    def test_file_not_found(self, tmp_path):
        """Test when kserve-deps.env doesn't exist"""
        version = _read_kserve_version(tmp_path)
        assert version is None

    def test_version_not_defined(self, tmp_path):
        """Test when KSERVE_VERSION is not in file"""
        deps_file = tmp_path / 'kserve-deps.env'
        deps_file.write_text('''
# Only other versions
ISTIO_VERSION=1.27.1
''')

        version = _read_kserve_version(tmp_path)
        assert version is None

    def test_comments_and_empty_lines(self, tmp_path):
        """Test handling of comments and empty lines"""
        deps_file = tmp_path / 'kserve-deps.env'
        deps_file.write_text('''
# This is a comment

# KServe
KSERVE_VERSION=v0.16.0
# Another comment
''')

        version = _read_kserve_version(tmp_path)
        assert version == 'v0.16.0'

    def test_whitespace_handling(self, tmp_path):
        """Test whitespace is stripped"""
        deps_file = tmp_path / 'kserve-deps.env'
        deps_file.write_text('KSERVE_VERSION=  v0.16.0  \n')

        version = _read_kserve_version(tmp_path)
        assert version == 'v0.16.0'
