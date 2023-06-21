FROM python:3.9-slim-bullseye

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY custom_transformer custom_transformer
RUN pip install --no-cache-dir -e ./custom_transformer -f https://download.pytorch.org/whl/torch_stable.html

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_transformer.model_grpc"]
