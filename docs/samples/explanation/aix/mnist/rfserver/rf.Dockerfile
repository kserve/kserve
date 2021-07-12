FROM python:3.7

COPY . .
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir kfserving==0.4.1
RUN pip install --no-cache-dir -e .
ENTRYPOINT ["python", "-m", "rfserver", "--model_name", "aixserver"]
