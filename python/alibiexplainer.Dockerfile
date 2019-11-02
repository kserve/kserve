FROM python:3.7

COPY . .
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN git clone -b 'v0.3.2' --single-branch https://github.com/SeldonIO/alibi.git && \
    cd alibi && \
    pip install .
RUN pip install -e ./alibiexplainer
ENTRYPOINT ["python", "-m", "alibiexplainer"]
