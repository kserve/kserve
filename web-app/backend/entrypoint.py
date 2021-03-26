import os
import sys

from apps import v1beta1
from kubeflow.kubeflow.crud_backend import config, logging

log = logging.getLogger(__name__)


APP_NAME = os.environ.get("APP_NAME", "Models Web App")
BACKEND_MODE = os.environ.get("BACKEND_MODE",
                              config.BackendMode.PRODUCTION.value)

PREFIX = os.environ.get("APP_PREFIX", "/")
APP_VERSION = os.environ.get("APP_VERSION", "v1beta1")

cfg = config.get_config(BACKEND_MODE)
cfg.PREFIX = PREFIX
cfg.APP_VERSION = APP_VERSION

# Load the app based on APP_VERSION env var
if APP_VERSION == "v1beta1":
    app = v1beta1.create_app(APP_NAME, cfg)
else:
    log.error("No app for: %s", APP_VERSION)
    sys.exit(1)


if __name__ == "__main__":
    app.run()
