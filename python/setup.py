from setuptools import setup, find_packages
import os

setup(
    name='kfserving',
    version='0.1.0',
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kubeflow/kfserving/model-servers/kfserver',
    description='Model Server for arbitrary python ML frameworks.',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages(),
    install_requires=[
        "tornado >= 1.4.1",
        "xgboost == 0.82",
        "scikit-learn == 0.20.3",
        "argparse >= 1.4.0"
    ],
)
