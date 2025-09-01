# Steps to generate

```bash
curl https://raw.githubusercontent.com/openai/openai-openapi/master/openapi.yaml -o openapi-2.0.0.yaml
datamodel-codegen --input openapi-2.0.0.yaml --input-file-type openapi --output openapi.py --output-model-type pydantic_v2.BaseModel --use-double-quotes --collapse-root-models  --enum-field-as-literal all --strict-nullable```

Adapted from the generated `openapi.py`
