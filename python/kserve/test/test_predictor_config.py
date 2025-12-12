from kserve.predictor_config import PredictorConfig


def test_timeout():
    p_config = PredictorConfig(
        predictor_host="localhost:8080", predictor_request_timeout_seconds=100
    )

    assert p_config.predictor_request_timeout_seconds == 100
    assert p_config.timeout == 100


def test_retries():
    p_config = PredictorConfig(
        predictor_host="localhost:8080", predictor_request_retries=5
    )

    assert p_config.predictor_request_retries == 5
    assert p_config.retries == 5


def test_use_ssl():
    p_config_true = PredictorConfig(
        predictor_host="localhost:8080", predictor_use_ssl=True
    )

    assert p_config_true.predictor_use_ssl is True
    assert p_config_true.use_ssl is True

    p_config_false = PredictorConfig(
        predictor_host="localhost:8080", predictor_use_ssl=False
    )

    assert p_config_false.predictor_use_ssl is False
    assert p_config_false.use_ssl is False
