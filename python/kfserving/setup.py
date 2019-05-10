from setuptools import setup, find_packages

setup(
    name='kfserver',
    version='0.1.0',
    author_email='ellisbigelow@google.com',
    license='../../LICENSE.txt',
    url='https://github.com/kubeflow/kfserving/python/kfserving/kfserving',
    description='Model Server for arbitrary python ML frameworks.',
    long_description=open('README.md').read(),
    python_requires='>3.4',
    packages=find_packages("kfserving"),
    install_requires=[
        "tornado >= 1.4.1",
        "argparse >= 1.4.0",
        "numpy"
    ],
    tests_require=[
        'pytest',
        'pytest-tornasync',
        'mypy'
    ]
)
