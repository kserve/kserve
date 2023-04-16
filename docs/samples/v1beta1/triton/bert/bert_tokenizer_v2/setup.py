import os

from setuptools import setup, find_packages

tests_require = [
    'pytest',
    'mypy'
]

with open(os.path.join(os.getcwd(), '../../../../../../python/VERSION')) as version_file:
    version = version_file.read().strip()

setup(
    name='bert_transformer_v2',
    version='0.1.0',
    author_email='dsun20@bloomberg.net',
    description='BertTransformerV2',
    python_requires='>=3.7',
    packages=find_packages("bert_transformer"),
    install_requires=[
        f"kserve>={version}",
        "tensorflow==2.7.2",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
