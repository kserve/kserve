FROM python:3.7

COPY alibiexplainer alibiexplainer
COPY kfserving kfserving
 
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN git clone https://github.com/SeldonIO/alibi.git && \
    cd alibi && \
    pip install .
RUN pip install -e ./alibiexplainer
ENTRYPOINT ["python", "-m", "alibiexplainer"]
