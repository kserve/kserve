FROM python:3.9-slim-bullseye

COPY third_party third_party

COPY kserve kserve
# pip 20.x breaks xgboost wheels https://github.com/dmlc/xgboost/issues/5221
RUN pip install --no-cache-dir pip==19.3.1 && pip install --no-cache-dir -e ./kserve

RUN apt-get update && apt-get install libgomp1

COPY xgbserver xgbserver
RUN pip install --no-cache-dir -e ./xgbserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "xgbserver"]
