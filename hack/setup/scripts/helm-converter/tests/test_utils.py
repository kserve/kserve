"""
Tests for utils module - cert-manager annotation processing
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

import pytest  # noqa: E402
from helm_converter.generators.utils import replace_cert_manager_namespace  # noqa: E402


class TestReplaceCertManagerNamespace:
    """Test replace_cert_manager_namespace function"""

    def test_inject_ca_from_replacement(self):
        """Test cert-manager.io/inject-ca-from annotation replacement"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/serving-cert',
            'other-annotation': 'value'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/serving-cert'
        assert result['other-annotation'] == 'value'

    def test_llmisvc_cert_replacement(self):
        """Test llmisvc cert annotation replacement"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/llmisvc-serving-cert'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/llmisvc-serving-cert'

    def test_localmodel_cert_replacement(self):
        """Test localmodel cert annotation replacement"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/localmodel-serving-cert'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/localmodel-serving-cert'

    def test_issuer_annotation(self):
        """Test cert-manager.io/issuer annotation replacement"""
        annotations = {
            'cert-manager.io/issuer': 'kserve/selfsigned-issuer'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/issuer'] == '{{ .Release.Namespace }}/selfsigned-issuer'

    def test_cluster_issuer_annotation(self):
        """Test cert-manager.io/cluster-issuer annotation (no namespace)"""
        annotations = {
            'cert-manager.io/cluster-issuer': 'letsencrypt-prod'
        }
        result = replace_cert_manager_namespace(annotations)
        # ClusterIssuer has no namespace, should remain unchanged
        assert result['cert-manager.io/cluster-issuer'] == 'letsencrypt-prod'

    def test_no_slash_in_value(self):
        """Test annotation value without slash - should remain unchanged"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'just-a-name'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == 'just-a-name'

    def test_multiple_slashes(self):
        """Test annotation value with multiple slashes - only first is namespace"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/cert/extra/path'
        }
        result = replace_cert_manager_namespace(annotations)
        # First slash separates namespace from rest
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/cert/extra/path'

    def test_none_annotations(self):
        """Test None annotations - should return None"""
        result = replace_cert_manager_namespace(None)
        assert result is None

    def test_empty_annotations(self):
        """Test empty annotations dict - should return empty dict"""
        result = replace_cert_manager_namespace({})
        assert result == {}

    def test_no_cert_manager_annotations(self):
        """Test annotations without cert-manager - should remain unchanged"""
        annotations = {
            'some-annotation': 'value',
            'another-annotation': 'another-value'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result == annotations

    def test_original_not_modified(self):
        """Test that original annotations dict is not modified"""
        original = {
            'cert-manager.io/inject-ca-from': 'kserve/serving-cert',
            'other': 'value'
        }
        original_copy = original.copy()
        result = replace_cert_manager_namespace(original)

        # Original should not be modified
        assert original == original_copy
        # Result should be modified
        assert result != original
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/serving-cert'

    def test_multiple_cert_manager_annotations(self):
        """Test multiple cert-manager annotations in same resource"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/serving-cert',
            'cert-manager.io/issuer': 'kserve/selfsigned-issuer',
            'other-annotation': 'value'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/serving-cert'
        assert result['cert-manager.io/issuer'] == '{{ .Release.Namespace }}/selfsigned-issuer'
        assert result['other-annotation'] == 'value'

    def test_webhook_configuration_annotation(self):
        """Test typical WebhookConfiguration annotation"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/serving-cert'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/serving-cert'

    def test_crd_conversion_webhook_annotation(self):
        """Test CRD conversion webhook annotation"""
        annotations = {
            'cert-manager.io/inject-ca-from': 'kserve/llmisvc-serving-cert'
        }
        result = replace_cert_manager_namespace(annotations)
        assert result['cert-manager.io/inject-ca-from'] == '{{ .Release.Namespace }}/llmisvc-serving-cert'


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
