
```
python -m sklearnserver --model_dir ./  --model_name income --protocol seldon.http
```

```
python -m alibiexplainer --model_url http://localhost:8080/models/income:predict --protocol seldon.http --training_data ./train.joblib --feature_names ./features.joblib --categorical_map ./category_map.joblib --http_port 8081
```


