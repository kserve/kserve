FROM registry.baidubce.com/paddlepaddle/serving:0.5.0-devel

RUN git clone https://github.com/PaddlePaddle/Serving.git

RUN pip install paddle-serving-server paddle-serving-app paddle-serving-client

RUN python -m paddle_serving_app.package --get_model lac && tar -xzvf lac.tar.gz

CMD python Serving/python/examples/lac/lac_web_service.py lac_model/ lac_workdir 8080
