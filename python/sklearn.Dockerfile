FROM python:3.7-slim

# gsutil required packages
RUN apt-get update && apt-get install -y curl lsb-release gnupg
RUN export CLOUD_SDK_REPO="cloud-sdk-$(lsb_release -c -s)" && echo "deb http://packages.cloud.google.com/apt $CLOUD_SDK_REPO main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
RUN curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
RUN apt-get update && apt-get install -y google-cloud-sdk

COPY . .
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./sklearnserver
COPY sklearnserver/model.joblib /tmp/models/model.joblib

ENTRYPOINT ["python", "-m", "sklearnserver"]
