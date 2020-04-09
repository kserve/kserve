import os
from flask import Flask, request

from bentoml import load


app = Flask(__name__)


bento_service = load('.')
api = bento_service.get_service_api('predict')


@app.route('/v1/models/iris-classifier:predict')
def predict():
    return api.handle_request(request)


if __name__ == "__main__":
    app.run(debug=True, host='0.0.0.0', port=int(os.environ.get('PORT', 8080)))
