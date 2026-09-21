import importlib.util
import json
import os
import pathlib
import signal
import tempfile
import unittest
from unittest import mock


ENTRYPOINT = pathlib.Path(__file__).with_name("capture-entrypoint.py")
SPEC = importlib.util.spec_from_file_location("capture_entrypoint", ENTRYPOINT)
capture_entrypoint = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(capture_entrypoint)


GROUPED_CAPTURE_CONFIG = {
    "version": 1,
    "cacheDir": "/workspace/cache/0",
    "targetImage": "registry.example/cache:session",
    "capture": {
        "name": "capture",
        "namespace": "team",
        "sessionID": "session-id",
    },
    "cachePaths": [
        {
            "containerName": "kserve-container",
            "containerPath": "/tmp/vllm",
            "ociPath": "io.vllm.cache",
        }
    ],
}
READINESS_CONFIG = {
    "url": "http://127.0.0.1:8080/health",
    "mcvCaptureReadinessTimeoutSeconds": 600,
}
RUNTIME_INFO = {
    "commandHash": "command-hash",
    "argsHash": "args-hash",
    "modelURIHash": "model-hash",
}


def grouped_environment():
    return {
        "MCV_CAPTURE_CONFIG": json.dumps(GROUPED_CAPTURE_CONFIG),
        "MCV_READINESS_CONFIG": json.dumps(READINESS_CONFIG),
        "MCV_RUNTIME_INFO": json.dumps(RUNTIME_INFO),
        "MCV_SOURCE_POD_NAME": "model-pod",
    }


class CaptureEntrypointTest(unittest.TestCase):
    def setUp(self):
        capture_entrypoint.shutdown_event.clear()

    def tearDown(self):
        capture_entrypoint.shutdown_event.clear()

    def test_grouped_configuration_is_read(self):
        with mock.patch.dict(os.environ, grouped_environment(), clear=True):
            self.assertEqual(
                GROUPED_CAPTURE_CONFIG,
                capture_entrypoint.capture_config(),
            )
            self.assertEqual(
                READINESS_CONFIG,
                capture_entrypoint.readiness_config(),
            )
            self.assertEqual(RUNTIME_INFO, capture_entrypoint.runtime_info())

    def test_grouped_configuration_is_required(self):
        for name, reader in (
            ("MCV_CAPTURE_CONFIG", capture_entrypoint.capture_config),
            ("MCV_READINESS_CONFIG", capture_entrypoint.readiness_config),
            ("MCV_RUNTIME_INFO", capture_entrypoint.runtime_info),
        ):
            with self.subTest(name=name), mock.patch.dict(os.environ, {}, clear=True):
                with self.assertRaisesRegex(
                    capture_entrypoint.CaptureConfigurationError, f"{name} is required"
                ):
                    reader()

    def test_invalid_capture_configuration_is_rejected(self):
        invalid_values = (
            "not-json",
            json.dumps({"version": 2}),
            json.dumps({**GROUPED_CAPTURE_CONFIG, "capture": "invalid"}),
        )
        for value in invalid_values:
            with self.subTest(value=value), mock.patch.dict(
                os.environ,
                {"MCV_CAPTURE_CONFIG": value},
                clear=True,
            ):
                with self.assertRaises(capture_entrypoint.CaptureConfigurationError):
                    capture_entrypoint.capture_config()

    def test_invalid_readiness_configuration_is_rejected(self):
        invalid_values = (
            {"url": 123},
            {**READINESS_CONFIG, "mcvCaptureReadinessTimeoutSeconds": 0},
        )
        for invalid in invalid_values:
            with self.subTest(invalid=invalid), mock.patch.dict(
                os.environ,
                {"MCV_READINESS_CONFIG": json.dumps(invalid)},
                clear=True,
            ):
                with self.assertRaises(capture_entrypoint.CaptureConfigurationError):
                    capture_entrypoint.readiness_config()

    def test_readiness_wait_stops_at_configured_timeout(self):
        with mock.patch.dict(
            os.environ,
            {"MCV_READINESS_CONFIG": json.dumps(READINESS_CONFIG)},
            clear=True,
        ), mock.patch.object(
            capture_entrypoint.time, "monotonic", side_effect=[0, 601]
        ), mock.patch.object(capture_entrypoint.urllib.request, "urlopen") as urlopen:
            with self.assertRaises(capture_entrypoint.ReadinessTimeout):
                capture_entrypoint.wait_for_readiness()

        urlopen.assert_not_called()

    def test_snapshot_retries_after_transient_failure(self):
        failed_process = mock.Mock()
        failed_process.poll.return_value = 4
        successful_process = mock.Mock()
        successful_process.poll.return_value = 0
        with mock.patch.object(
            capture_entrypoint.subprocess,
            "Popen",
            side_effect=[failed_process, successful_process],
        ) as run, mock.patch.object(
            capture_entrypoint.shutdown_event, "wait", return_value=False
        ) as wait:
            capture_entrypoint.create_snapshot("/workspace/cache/0")

        self.assertEqual(2, run.call_count)
        wait.assert_called_once_with(
            capture_entrypoint.SNAPSHOT_RETRY_DELAY_SECONDS
        )

    def test_snapshot_fails_after_retry_limit(self):
        failure = mock.Mock()
        failure.poll.return_value = 4
        with mock.patch.object(
            capture_entrypoint.subprocess,
            "Popen",
            return_value=failure,
        ) as run, mock.patch.object(
            capture_entrypoint.shutdown_event, "wait", return_value=False
        ) as wait:
            with self.assertRaisesRegex(
                RuntimeError,
                f"after {capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS} attempts",
            ):
                capture_entrypoint.create_snapshot("/workspace/cache/0")

        self.assertEqual(capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS, run.call_count)
        self.assertEqual(
            capture_entrypoint.SNAPSHOT_RETRY_ATTEMPTS - 1,
            wait.call_count,
        )

    def test_idle_wait_stops_when_shutdown_is_requested(self):
        with mock.patch.object(
            capture_entrypoint.shutdown_event, "wait", return_value=True
        ) as wait:
            capture_entrypoint.wait_for_shutdown()

        wait.assert_called_once_with(
            capture_entrypoint.SHUTDOWN_POLL_INTERVAL_SECONDS
        )

    def test_shutdown_signal_sets_event(self):
        capture_entrypoint.handle_shutdown(signal.SIGTERM, None)

        self.assertTrue(capture_entrypoint.shutdown_event.is_set())

    def test_command_is_terminated_as_a_process_group(self):
        process = mock.Mock(pid=1234)
        process.poll.side_effect = [None, None]

        def request_shutdown(_timeout):
            capture_entrypoint.shutdown_event.set()
            return True

        with mock.patch.object(
            capture_entrypoint.subprocess, "Popen", return_value=process
        ) as popen, mock.patch.object(
            capture_entrypoint.shutdown_event, "wait", side_effect=request_shutdown
        ), mock.patch.object(capture_entrypoint.os, "killpg") as killpg:
            with self.assertRaises(capture_entrypoint.ShutdownRequested):
                capture_entrypoint.run_command(["/mcv", "--create"])

        popen.assert_called_once_with(["/mcv", "--create"], start_new_session=True)
        killpg.assert_called_once_with(1234, signal.SIGTERM)
        process.wait.assert_called_once_with(
            timeout=capture_entrypoint.COMMAND_TERMINATION_TIMEOUT_SECONDS
        )

    def test_capture_result_uses_grouped_metadata(self):
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as file:
            json.dump({"state": "Succeeded"}, file)
            file.flush()
            with mock.patch.dict(os.environ, grouped_environment(), clear=True), mock.patch.object(
                capture_entrypoint, "RESULT_PATH", file.name
            ):
                result = capture_entrypoint.capture_result()

        self.assertEqual(
            json.dumps(GROUPED_CAPTURE_CONFIG["cachePaths"], separators=(",", ":")),
            result["cachePaths"],
        )
        self.assertEqual(
            json.dumps(RUNTIME_INFO, separators=(",", ":"), sort_keys=True),
            result["runtimeInfo"],
        )
        self.assertEqual("model-pod", result["sourcePodName"])
        self.assertEqual("session-id", result["captureSessionID"])

    def test_report_does_not_retry_configuration_errors(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureConfigurationError("invalid"),
        ) as reporter_request:
            capture_entrypoint.report({"state": "Failed"})

        reporter_request.assert_called_once_with({"state": "Failed"})

    def test_capture_session_superseded_detection_requires_matching_status(self):
        cases = (
            (
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Invalid",
                    "code": 422,
                    "message": "testing value /status/activeSession/id failed: test failed",
                },
                True,
            ),
            (
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Invalid",
                    "code": 422,
                    "message": "testing value /status/activeSession/podName failed: test failed",
                },
                True,
            ),
            (
                {
                    "kind": "Status",
                    "status": "Failure",
                    "reason": "Invalid",
                    "code": 422,
                    "message": "admission webhook rejected request: test failed",
                },
                False,
            ),
            ("not-json", False),
        )
        for response, expected in cases:
            with self.subTest(response=response):
                body = response if isinstance(response, str) else json.dumps(response)
                self.assertEqual(
                    expected,
                    capture_entrypoint.is_capture_session_superseded(422, body),
                )

    def test_report_stops_when_capture_session_is_superseded(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureSessionSuperseded(),
        ):
            with self.assertRaises(capture_entrypoint.CaptureSessionSuperseded):
                capture_entrypoint.report({"state": "Capturing"})

    def test_report_stops_when_capture_target_is_deleted(self):
        with mock.patch.object(
            capture_entrypoint,
            "reporter_request",
            side_effect=capture_entrypoint.CaptureTargetGone(),
        ):
            with self.assertRaises(capture_entrypoint.CaptureTargetGone):
                capture_entrypoint.report({"state": "Capturing"})


if __name__ == "__main__":
    unittest.main()
