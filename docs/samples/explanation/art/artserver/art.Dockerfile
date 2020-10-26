FROM python:3.7

COPY ./artserver/example_model /tmp/model
COPY . .
RUN pip install --upgrade pip && pip install kfserving==0.4.0
RUN pip install -e .
ENTRYPOINT ["python", "-m", "artserver", "--model_dir", "example_model/"]