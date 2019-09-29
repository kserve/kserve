import time
import os
from kubeflow.testing import util

def test_tensorflow_kfservice():
    app_dir = os.path.dirname(__file__)
    app_dir = os.path.abspath(app_dir)
    util.run(['kubectl', 'create', '-f', 'tensorflow.yaml',
              '-n', 'kfserving-ci-test'], cwd=app_dir)
    for i in range(600):
        result = util.run(['kubectl', 'get', 'inferenceservice', 'flowers-sample',
                           '-n', 'kfserving-ci-test'], cwd=app_dir)
        if 'True' in result:
            return
        else:
            util.run(['kubectl', 'describe', 'inferenceservice', 'flowers-sample',
                      '-n', 'kfserving-ci-test'], cwd=app_dir)
            time.sleep(10)
    raise RuntimeError("Timeout to start the tensorflow KFService.")