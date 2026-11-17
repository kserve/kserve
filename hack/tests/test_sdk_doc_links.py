"""Generated model references must resolve without changing SDK model types."""

import importlib.util
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location(
    "sdk_doc_links", Path(__file__).parents[1] / "python-sdk/fix-doc-links.py"
)
doc_links = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(doc_links)


class SDKDocumentationTests(unittest.TestCase):
    def test_resolves_imported_and_source_types_and_preserves_generated_pages(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs = root / "python/kserve/docs"
            docs.mkdir(parents=True)
            (docs / "Existing.md").write_text("# Existing\n")
            source = root / "pkg/apis/serving/v1alpha1/types.go"
            source.parent.mkdir(parents=True)
            source.write_text("package v1alpha1\n\ntype ExampleSpec struct {}\n")
            document = docs / "Example.md"
            document.write_text(
                "[local](Existing.md) [spec](V1alpha1ExampleSpec.md) [pod](V1PodOS.md) [**object**](.md)\n"
            )
            doc_links.rewrite_docs(root, {})
            rewritten = document.read_text()
            self.assertIn("(Existing.md)", rewritten)
            self.assertIn("(../../../pkg/apis/serving/v1alpha1/types.go)", rewritten)
            self.assertIn("(https://pkg.go.dev/k8s.io/api/core/v1#PodOS)", rewritten)
            self.assertIn("**object**", rewritten)
            self.assertNotIn("(.md)", rewritten)
            doc_links.rewrite_docs(root, {})
            self.assertEqual(document.read_text(), rewritten)

    def test_upstream_references_use_the_repository_module_versions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs = root / "python/kserve/docs"
            docs.mkdir(parents=True)
            document = docs / "Example.md"
            document.write_text(
                "[core](V1PodOS.md) [keda](GithubComKedacoreKedaV2ApisKedaV1alpha1ScaleTriggers.md) [quantity](ResourceInt64Amount.md) [imported](V1Container.md)\n"
            )
            doc_links.rewrite_docs(
                root,
                {"V1Container": "from kubernetes.client import V1Container"},
                versions={
                    "k8s.io/api": "v0.37.1",
                    "k8s.io/apimachinery": "v0.37.1",
                    "github.com/kedacore/keda/v2": "v2.20.2",
                },
            )
            text = document.read_text()
            self.assertIn("k8s.io/api@v0.37.1/core/v1#PodOS", text)
            self.assertIn("github.com/kedacore/keda/v2@v2.20.2/apis", text)
            self.assertIn("kubernetes/apimachinery/blob/v0.37.1/", text)
            self.assertIn(
                "kubernetes-client/python/blob/master/kubernetes/docs/V1Container.md",
                text,
            )

    def test_source_reference_does_not_churn_when_type_line_moves(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs = root / "python/kserve/docs"
            docs.mkdir(parents=True)
            source = root / "pkg/apis/serving/v1alpha1/types.go"
            source.parent.mkdir(parents=True)
            source.write_text("type ExampleSpec struct {}\n")
            document = docs / "Example.md"
            document.write_text("[spec](V1alpha1ExampleSpec.md)\n")
            doc_links.rewrite_docs(root, {})
            before = document.read_text()
            source.write_text("\n\ntype ExampleSpec struct {}\n")
            document.write_text("[spec](V1alpha1ExampleSpec.md)\n")
            doc_links.rewrite_docs(root, {})
            self.assertEqual(document.read_text(), before)

    def test_unresolved_model_fails_before_any_document_is_changed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs = root / "python/kserve/docs"
            docs.mkdir(parents=True)
            document = docs / "Example.md"
            original = "[pod](V1PodOS.md) [unknown](MissingModel.md)\n"
            document.write_text(original)
            with self.assertRaisesRegex(ValueError, "MissingModel"):
                doc_links.rewrite_docs(root, {})
            self.assertEqual(document.read_text(), original)

    def test_unknown_kubernetes_type_cannot_be_disguised_as_a_working_page(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docs = root / "python/kserve/docs"
            docs.mkdir(parents=True)
            (docs / "Example.md").write_text("[unknown](V1NotAKnownType.md)\n")
            with self.assertRaisesRegex(ValueError, "V1NotAKnownType"):
                doc_links.rewrite_docs(root, {})
