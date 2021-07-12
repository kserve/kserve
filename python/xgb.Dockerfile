FROM python:3.7-slim

RUN apt-get update && apt-get install libgomp1

COPY xgbserver xgbserver
COPY kfserving kfserving
COPY third_party third_party

# pip 20.x breaks xgboost wheels https://github.com/dmlc/xgboost/issues/5221
RUN pip install --no-cache-dir pip==19.3.1 && pip install --no-cache-dir -e ./kfserving
RUN pip install --no-cache-dir -e ./xgbserver
ENTRYPOINT ["python", "-m", "xgbserver"]
