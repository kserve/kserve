ARG BASE_IMAGE=python:3.9-slim-bullseye
FROM $BASE_IMAGE

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY custom_model custom_model
RUN pip install -r ./custom_model/requirements.txt

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_model"]
