FROM python:3.7-slim

RUN apt-get update && apt-get install -y libgomp1 curl lsb-release gnupg

# gsutil required packages
RUN export CLOUD_SDK_REPO="cloud-sdk-$(lsb_release -c -s)" && echo "deb http://packages.cloud.google.com/apt $CLOUD_SDK_REPO main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
RUN curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
RUN apt-get update && apt-get install -y google-cloud-sdk


COPY . .
RUN pip install -e ./kfserving && pip install -e ./xgbserver
COPY model.bst /tmp/models/model.bst

ENTRYPOINT ["python", "-m", "xgbserver"]
