FROM python:3.9-slim

RUN apt-get update && apt-get install -y libglib2.0-0
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir kserve
COPY . .
RUN pip install --no-cache-dir -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
