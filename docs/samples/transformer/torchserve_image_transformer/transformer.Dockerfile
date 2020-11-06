FROM python:3.7-slim
ARG BRANCH_NAME_KF=master

COPY . .
RUN pip install --upgrade pip 
RUN git clone -b ${BRANCH_NAME_KF} https://github.com/kubeflow/kfserving.git
RUN pip install -e ./kfserving/python/kfserving 
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
