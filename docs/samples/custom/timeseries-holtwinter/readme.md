#Steps in Building the Image:

1.Docker Build
2.Docker Push
3.Apply the Image using the custom.yaml (kubectl apply -f custom.yaml)


Curl Request:
curl -v -H "Host: sales-application.kfserving.169.46.83.189.xip.io" http://169.46.83.189:31380/v1/models/sales-application:predict  -d '{"instances": [{ "image": {"weeks":"3"}}]}'


Replace 3 with the number that you want a prediction for.


Sample output:

* About to connect() to 169.46.83.189 port 31380 (#0)
*   Trying 169.46.83.189...
* Connected to 169.46.83.189 (169.46.83.189) port 31380 (#0)
> POST /v1/models/sales-application:predict HTTP/1.1
> User-Agent: curl/7.29.0
> Accept: */*
> Host: sales-application.kfserving.169.46.83.189.xip.io
> Content-Length: 42
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 42 out of 42 bytes
< HTTP/1.1 200 OK
< content-length: 290
< content-type: application/json; charset=UTF-8
< date: Mon, 21 Sep 2020 09:57:08 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 147
< 
* Connection #0 to host 169.46.83.189 left intact
{"predictions": {"sales": "application", "forecast_weeks": "3", "predicted_values_holt": "[17772.43560134 17456.2060787  18356.3081933 ]", "predicted_values_arima": "[17111.90505762 16448.25712033 16503.57877108]", "predicted_values_lstm": "[17939.65913346 17951.75847989 17952.62518856]"}}


