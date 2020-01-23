FROM python:3.7-slim

RUN apt-get update && apt-get install libgomp1

COPY xgbserver xgbserver
COPY kfserving kfserving

# pip 20.x breaks xgboost wheels https://github.com/dmlc/xgboost/issues/5221
# RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./kfserving
RUN pip install -e ./xgbserver
ENTRYPOINT ["python", "-m", "xgbserver"]
