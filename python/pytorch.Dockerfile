FROM pytorch/pytorch:latest

COPY . .
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./pytorchserver
ENTRYPOINT ["python", "-m", "pytorchserver"]
