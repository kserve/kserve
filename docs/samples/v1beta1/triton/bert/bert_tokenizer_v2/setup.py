from setuptools import setup, find_packages

tests_require = [
    'pytest',
    'pytest-tornasync',
    'mypy'
]

setup(
    name='bert_transformer_v2',
    version='0.1.0',
    author_email='dsun20@bloomberg.net',
    description='BertTransformerV2',
    python_requires='>=3.7',
    packages=find_packages("bert_transformer"),
    install_requires=[
        "kserve>=0.9.0",
        "tensorflow==2.7.2",
    ],
    tests_require=tests_require,
    extras_require={'test': tests_require}
)
