from setuptools import setup, find_packages

tests_require = [
    'pytest',
    'pytest-tornasync',
    'mypy'
]

setup(
    name='bert_transformer',
    version='0.1.0',
    author_email='dsun20@bloomberg.net',
    description='BertTransformer',
    python_requires='>=3.6',
    packages=find_packages("bert_transformer"),
    install_requires=[
        "kfserving>=0.3.0",
        "tensorflow==2.4.2",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
