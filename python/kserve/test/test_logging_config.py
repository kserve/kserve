import logging
import sys
import io
import pytest


@pytest.fixture(autouse=True)
def _isolate_logging(monkeypatch):
    """
    Isolate logging across tests by resetting root handlers and the 'kserve'
    logger between tests to avoid cross-test contamination.
    """
    # Backup existing handlers
    root = logging.getLogger()
    original_handlers = list(root.handlers)
    for h in original_handlers:
        root.removeHandler(h)
    # Ensure 'kserve' logger also has no handlers initially
    klog = logging.getLogger("kserve")
    k_handlers = list(klog.handlers)
    for h in k_handlers:
        klog.removeHandler(h)
    # Reset level/propagation so default behavior inherits from root
    klog.setLevel(logging.NOTSET)
    klog.propagate = True

    yield

    # Restore handlers (best-effort)
    for h in list(root.handlers):
        root.removeHandler(h)
    for h in original_handlers:
        root.addHandler(h)
    for h in list(klog.handlers):
        klog.removeHandler(h)
    for h in k_handlers:
        klog.addHandler(h)


def _get_kserve_logging_module():
    # Import from the installed editable package
    from kserve import logging as klog_mod  # type: ignore

    return klog_mod


def test_respect_existing_logging(monkeypatch):
    klog_mod = _get_kserve_logging_module()

    fmt = "USERFMT|%(levelname)s|%(name)s|%(message)s"
    buf = io.StringIO()
    # Attach a user-defined handler directly to the 'kserve' logger
    klog = logging.getLogger("kserve")
    h = logging.StreamHandler(buf)
    h.setFormatter(logging.Formatter(fmt))
    klog.addHandler(h)
    klog.setLevel(logging.INFO)
    klog.propagate = False
    # Sanity: hasHandlers should be True now
    assert klog.hasHandlers()

    klog_mod.configure_logging(None)

    log = logging.getLogger("kserve")
    # Should still have the same direct handler and not be overridden
    assert (
        log.handlers and log.handlers[0] is h
    ), "existing user handler should be preserved"
    log.info("RESPECT_TEST")
    contents = buf.getvalue()
    assert "USERFMT|INFO|kserve|RESPECT_TEST" in contents


def test_apply_default_when_not_configured(monkeypatch):
    klog_mod = _get_kserve_logging_module()

    # No basicConfig here; expect KServe default formatter on stderr
    buf = io.StringIO()
    monkeypatch.setattr(sys, "stderr", buf, raising=False)
    # Ensure kserve logger does not see ancestor handlers so configure_logging runs
    klog = logging.getLogger("kserve")
    klog.propagate = False
    klog_mod.configure_logging(None)

    log = logging.getLogger("kserve")
    # After configuration, kserve logger should have a StreamHandler bound to sys.stderr
    assert (
        log.handlers
    ), "kserve logger should have handlers after default configuration"
    log.info("DEFAULT_TEST")
    contents = buf.getvalue()
    # Default goes to stderr per handler config
    assert "DEFAULT_TEST" in contents
    # Should include filename and function hints from KSERVE_LOGGER_FORMAT
    assert "test_logging_config.py:test_apply_default_when_not_configured" in contents


def test_explicit_config_overrides(monkeypatch):
    klog_mod = _get_kserve_logging_module()
    KSERVE_LOG_CONFIG = klog_mod.KSERVE_LOG_CONFIG

    cfg = {**KSERVE_LOG_CONFIG}
    cfg = {k: (v.copy() if isinstance(v, dict) else v) for k, v in cfg.items()}
    cfg["formatters"]["kserve"]["fmt"] = "EXPLFMT|%(levelname)s|%(message)s"

    buf = io.StringIO()
    monkeypatch.setattr(sys, "stderr", buf, raising=False)
    # Ensure kserve logger does not see ancestor handlers
    logging.getLogger("kserve").propagate = False
    klog_mod.configure_logging(cfg)

    log = logging.getLogger("kserve")
    log.info("EXPLICIT_TEST")
    contents = buf.getvalue()
    assert "EXPLFMT|INFO|EXPLICIT_TEST" in contents
