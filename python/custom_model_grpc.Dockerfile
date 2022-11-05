FROM python:3.9-slim-bullseye

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve && pip install --no-cache-dir torchvision

COPY custom_model custom_model

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_model.model_grpc"]
