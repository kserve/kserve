FROM python:3.7-slim

RUN apt-get update && apt-get install libgomp1

COPY xgbserver xgbserver
COPY kfserving kfserving

RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./xgbserver
ENTRYPOINT ["python", "-m", "xgbserver"]
