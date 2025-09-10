import sys
import types
import pathlib
import importlib.util
import pytest

torch = pytest.importorskip("torch", reason="torch not installed")

# stub kserve.logging
if "kserve.logging" not in sys.modules:
    kserve_pkg = types.ModuleType("kserve")
    kserve_logging = types.ModuleType("kserve.logging")

    class _DummyLogger:
        def info(self, *a, **k):
            pass

        def warning(self, *a, **k):
            pass

        def error(self, *a, **k):
            pass

    kserve_logging.logger = _DummyLogger()
    sys.modules["kserve"] = kserve_pkg
    sys.modules["kserve.logging"] = kserve_logging

UTILS_PATH = (
    pathlib.Path(__file__).resolve().parents[1] / "huggingfaceserver" / "utils.py"
)
spec = importlib.util.spec_from_file_location("hf_utils", UTILS_PATH)
hf_utils = importlib.util.module_from_spec(spec)
spec.loader.exec_module(hf_utils)
_mean_pooling = hf_utils._mean_pooling


def test_mean_pooling_none_mask_cpu():
    x = torch.ones(1, 3, 4)  # [B,S,H]
    out = _mean_pooling((x,), None)
    assert torch.allclose(out, torch.ones(1, 4))
