FROM python:3.7

COPY artexplainer artexplainer
COPY kfserving kfserving

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kfserving
RUN pip install --no-cache-dir -e ./artexplainer
ENTRYPOINT ["python", "-m", "artserver"]
