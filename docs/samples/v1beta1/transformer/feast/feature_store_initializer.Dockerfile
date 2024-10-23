FROM python:3.13-slim
RUN pip install --upgrade pip && pip install "feast[gcp]~=0.30.0" "feast[redis]~=0.30.0" "feast[aws]~=0.30.0"
WORKDIR feature_store_initializer
COPY feature_store_initializer_entrypoint.sh feature_store_initializer_entrypoint.sh
RUN chmod +x feature_store_initializer_entrypoint.sh
ENTRYPOINT ["./feature_store_initializer_entrypoint.sh"]
