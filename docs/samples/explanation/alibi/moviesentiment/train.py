import numpy as np
from sklearn.feature_extraction.text import CountVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score
from sklearn.model_selection import train_test_split
from alibi.datasets import fetch_movie_sentiment
from sklearn.pipeline import Pipeline
import joblib
from alibi.explainers import AnchorText
import spacy
from alibi.utils.download import spacy_model

# load data
movies = fetch_movie_sentiment()
movies.keys()
data = movies.data
labels = movies.target
target_names = movies.target_names

# define train and test set
np.random.seed(0)
train, test, train_labels, test_labels = train_test_split(data, labels, test_size=.2, random_state=42)
train, val, train_labels, val_labels = train_test_split(train, train_labels, test_size=.1, random_state=42)
train_labels = np.array(train_labels)
test_labels = np.array(test_labels)
val_labels = np.array(val_labels)

# define and  train an cnn model
vectorizer = CountVectorizer(min_df=1)
clf = LogisticRegression(solver='liblinear')
pipeline = Pipeline([('preprocess', vectorizer), ('clf', clf)])

print('Training ...')
pipeline.fit(train, train_labels)
print('Training done!')

preds_train = pipeline.predict(train)
preds_val = pipeline.predict(val)
preds_test = pipeline.predict(test)
print('Train accuracy', accuracy_score(train_labels, preds_train))
print('Validation accuracy', accuracy_score(val_labels, preds_val))
print('Test accuracy', accuracy_score(test_labels, preds_test))

print("Saving Model to model.joblib")
joblib.dump(pipeline, "model.joblib")

print("Creating Anchor Text explainer")
spacy_language_model = 'en_core_web_md'
spacy_model(model=spacy_language_model)
nlp = spacy.load(spacy_language_model)
anchors_text = AnchorText(nlp, lambda x: pipeline.predict(x))

# Test explanations locally
expl = anchors_text.explain("the actors are very bad")
print(expl)
