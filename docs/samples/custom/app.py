import os

from flask import Flask

app = Flask(__name__)


@app.route('/v1/models/custom-sample:predict')
def hello_world():
    greeting_target = os.environ.get('GREETING_TARGET', 'World')
    return 'Hello {}!\n'.format(greeting_target)


if __name__ == "__main__":
    app.run(debug=True, host='0.0.0.0', port=int(os.environ.get('PORT', 8080)))
