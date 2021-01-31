import argparse
import json
import base64
import grpc

from tensorflow.contrib.util import make_tensor_proto
from tensorflow_serving.apis import predict_pb2
from tensorflow_serving.apis import prediction_service_pb2_grpc


def predict(host, port, hostname, model, signature_name, input_path):
    # If hostname not set, we assume the host is a valid knative dns.
    if hostname:
        host_option = (('grpc.ssl_target_name_override', hostname,),)
    else:
        host_option = None
    channel = grpc.insecure_channel(target='{host}:{port}'.format(host=host, port=port), options=host_option)
    stub = prediction_service_pb2_grpc.PredictionServiceStub(channel)
    with open(input_path) as json_file:
        data = json.load(json_file)
    image = data['instances'][0]['image_bytes']['b64']
    key = data['instances'][0]['key']

    # Call classification model to make prediction
    request = predict_pb2.PredictRequest()
    request.model_spec.name = model
    request.model_spec.signature_name = signature_name
    image = base64.b64decode(image)
    request.inputs['image_bytes'].CopyFrom(
        make_tensor_proto(image, shape=[1]))
    request.inputs['key'].CopyFrom(make_tensor_proto(key, shape=[1]))

    result = stub.Predict(request, 10.0)
    print(result)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--host', help='Ingress Host Name', default='localhost', type=str)
    parser.add_argument('--port', help='Ingress Port', default=80, type=int)
    parser.add_argument('--model', help='TensorFlow Model Name', type=str)
    parser.add_argument('--signature_name', help='Signature name of saved TensorFlow model',
                        default='serving_default', type=str)
    parser.add_argument('--hostname', help='Service Host Name', default='', type=str)
    parser.add_argument('--input_path', help='Prediction data input path', default='./input.json', type=str)

    args = parser.parse_args()
    predict(args.host, args.port, args.hostname, args.model, args.signature_name, args.input_path)
