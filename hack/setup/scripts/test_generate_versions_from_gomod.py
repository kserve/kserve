import importlib.util
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).with_name("generate-versions-from-gomod.py")
SPEC = importlib.util.spec_from_file_location("generate_versions", SCRIPT)
generate_versions = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(generate_versions)


class TestEnsureHelmRepo(unittest.TestCase):
    def test_retries_transient_helm_failure(self):
        run = unittest.mock.Mock(
            side_effect=[
                "[]",
                subprocess.CalledProcessError(
                    1, "helm repo add istio https://example.com"
                ),
                "",
            ]
        )

        with (
            patch.object(generate_versions, "run", run),
            patch.object(generate_versions.time, "sleep"),
        ):
            generate_versions.ensure_helm_repo("istio", "https://example.com")

        self.assertEqual(run.call_count, 3)


if __name__ == "__main__":
    unittest.main()
