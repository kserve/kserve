FROM python:3.7-slim

COPY . .
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir kfserving
RUN pip install --no-cache-dir -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
