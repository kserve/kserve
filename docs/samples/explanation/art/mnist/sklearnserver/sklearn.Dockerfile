FROM python:3.7-slim

COPY . sklearnserver

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir kfserving
RUN pip install --no-cache-dir -e ./sklearnserver

ENTRYPOINT ["python", "-m", "sklearnserver", "--model_name", "artserver", "--model_dir", "file://sklearnserver/sklearnserver/example_model"]
