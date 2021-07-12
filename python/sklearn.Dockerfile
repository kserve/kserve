FROM python:3.7-slim

COPY sklearnserver sklearnserver
COPY kfserving kfserving

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kfserving
RUN pip install --no-cache-dir -e ./sklearnserver
COPY third_party third_party

ENTRYPOINT ["python", "-m", "sklearnserver"]
