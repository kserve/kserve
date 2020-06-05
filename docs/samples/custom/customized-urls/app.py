import os

from flask import Flask

app = Flask(__name__)
@app.route('/customized/urls/demo/test/1')
def customized_urls_test():
    target = os.environ.get('TARGET', 'World')
    return 'Hello {}. Func customized_urls_test is called!\n'.format(target)

@app.route('/v1/models/customized-sample:predict')
def hello_world():
    target = os.environ.get('TARGET', 'World')
    return 'Hello {}!\n'.format(target)

if __name__ == "__main__":
    app.run(debug=True,host='0.0.0.0',port=int(os.environ.get('PORT', 8080)))
