import pandas as pd
from sklearn.base import TransformerMixin


class DictToDFTransformer(TransformerMixin):

    def transform(self, X, y=None):
        return pd.DataFrame(X)

    def fit(self, X, y=None):
        return self
