FROM python:3.9-slim-bullseye

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY sklearnserver sklearnserver
RUN pip install --no-cache-dir -e ./sklearnserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "sklearnserver"]
