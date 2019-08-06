
model.joblib features.joblib category_map.joblib:
	python train.py

.PHONY: train
train: model.joblib features.joblib category_map.joblib

.PHONY: clean
clean:
	rm -f *.joblib
	rm -f *.dill
