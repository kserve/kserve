
model.joblib features.joblib category_map.joblib:
	python train.py

.PHONY: train
train: model.joblib features.joblib category_map.joblib

.PHONY: run_predictor
run_predictor: model.joblib
	python -m sklearnserver --model_dir ./  --model_name income --protocol seldon.http

.PHONY: clean
clean:
	rm -f *.joblib
	rm -f *.dill
