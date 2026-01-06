import sys
import pytest
import importlib
from unittest.mock import patch
from unittest.mock import MagicMock

def test_ray_serve_missing(monkeypatch):
    """Test ImportError when ray.serve is unavailable"""
    mock_ray = MagicMock()
    monkeypatch.setitem(sys.modules, 'ray', mock_ray)
    monkeypatch.setitem(sys.modules, 'ray.serve', None)
    monkeypatch.setitem(sys.modules, 'ray.serve.handle', None)
    
    module_name = 'kserve.kserve.ray'
    sys.modules.pop(module_name, None)
    
    with pytest.raises(ImportError):
        importlib.import_module(module_name)

def test_ray_serve_handle_missing(monkeypatch):
    """Test ImportError when ray.serve.handle is unavailable"""
    mock_ray = MagicMock()
    mock_serve = MagicMock()
    monkeypatch.setitem(sys.modules, 'ray', mock_ray)
    monkeypatch.setitem(sys.modules, 'ray.serve', mock_serve)
    monkeypatch.setitem(sys.modules, 'ray.serve.handle', None)
    
    module_name = 'kserve.kserve.ray'
    sys.modules.pop(module_name, None)
    
    with pytest.raises(ImportError):
        importlib.import_module(module_name)