from prometheus_client import Histogram

PROM_LABELS = ['model_name']
PRE_HIST_TIME = Histogram('request_preprocess_seconds', 'pre-process request latency', PROM_LABELS)
POST_HIST_TIME = Histogram('request_postprocess_seconds', 'post-process request latency', PROM_LABELS)
PREDICT_HIST_TIME = Histogram('request_predict_seconds', 'predict request latency', PROM_LABELS)
EXPLAIN_HIST_TIME = Histogram('request_explain_seconds', 'explain request latency', PROM_LABELS)


def get_labels(model_name):
    return {PROM_LABELS[0]: model_name}
