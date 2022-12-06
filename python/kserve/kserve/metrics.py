from prometheus_client import Histogram
import os

PROM_KEYS = {"K_SERVICE": "service_name", "K_CONFIGURATION": "configuration_name",
             "K_REVISION": "revision_name"}
PRE_HIST_TIME = Histogram('request_preprocess_seconds', 'pre-process request latency', PROM_KEYS.values())
POST_HIST_TIME = Histogram('request_postprocess_seconds', 'post-process request latency', PROM_KEYS.values())
PREDICT_HIST_TIME = Histogram('request_predict_seconds', 'predict request latency', PROM_KEYS.values())
EXPLAIN_HIST_TIME = Histogram('request_explain_seconds', 'explain request latency', PROM_KEYS.values())

# get_labels adds the service, configuration, and revision labels from the container onto the prometheus metric.
# this way, when the metrics are exported, they can be queried with the same labels that the
# queue-proxy has. https://github.com/knative/serving/blob/026291c4a46eea99cba66beadbc2180b35ab433d/pkg/metrics/key.go
def get_labels():
    labels = {}
    for env_key, tag in PROM_KEYS.items():
        # if the environment variable doesn't exist/is empty, we are not in serverless mode and will set the label to
        # "".
        labels[tag] = os.environ.get(env_key, "")
    return labels
