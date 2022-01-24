FROM python:3.7-slim

RUN apt-get update && apt-get install libgomp1

COPY xgbserver xgbserver
COPY kserve kserve
COPY third_party third_party

# pip 20.x breaks xgboost wheels https://github.com/dmlc/xgboost/issues/5221
RUN pip install --no-cache-dir pip==19.3.1 && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./xgbserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "xgbserver"]
