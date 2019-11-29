FROM yuzisun/kfserving:latest

COPY . .
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
