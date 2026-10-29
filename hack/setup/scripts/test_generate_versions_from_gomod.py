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

        sleep = unittest.mock.Mock()
        with (
            patch.object(generate_versions, "run", run),
            patch.object(generate_versions.time, "sleep", sleep),
        ):
            generate_versions.ensure_helm_repo("istio", "https://example.com")

        self.assertEqual(run.call_count, 3)
        self.assertEqual(sleep.call_count, 1)
        sleep.assert_called_once_with(2)

    def test_propagates_error_after_exhausted_retries(self):
        run = unittest.mock.Mock(
            side_effect=[
                "[]",
                subprocess.CalledProcessError(
                    1, "helm repo add istio https://example.com"
                ),
                subprocess.CalledProcessError(
                    1, "helm repo add istio https://example.com"
                ),
                subprocess.CalledProcessError(
                    1, "helm repo add istio https://example.com"
                ),
            ]
        )

        sleep = unittest.mock.Mock()
        with (
            patch.object(generate_versions, "run", run),
            patch.object(generate_versions.time, "sleep", sleep),
            self.assertRaises(subprocess.CalledProcessError),
        ):
            generate_versions.ensure_helm_repo("istio", "https://example.com")

        self.assertEqual(run.call_count, 4)
        # Delay between the three attempts, but never after the final failure.
        self.assertEqual(sleep.call_count, 2)
        sleep.assert_has_calls([unittest.mock.call(2), unittest.mock.call(2)])


if __name__ == "__main__":
    unittest.main()
