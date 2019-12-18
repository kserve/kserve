FROM python:3.7

COPY . .
RUN apt-get update
RUN pip install --upgrade pip && pip install -e ./kfserving
# Latest scipy has dependency issue when used with pytorch: https://github.com/scipy/scipy/issues/11237
RUN pip install scipy==1.3.3 
RUN pip install alibi==0.3.2
RUN pip install -e ./alibiexplainer
ENTRYPOINT ["python", "-m", "alibiexplainer"]
