import numpy as np
import requests
from alibi.datasets import fetch_adult
import pandas as pd
import plotly.graph_objects as go
from IPython.display import display, Markdown, display

def getFeatures(X,cmap):
    return pd.DataFrame(X).replace(cmap).values.squeeze().tolist()

def predict(X, name, ds, svc_hostname, cluster_ip):
    formData = {
    'instances': X
    }
    headers = {}
    headers["Host"] = svc_hostname
    res = requests.post('http://'+cluster_ip+'/v1/models/'+name+':predict', json=formData, headers=headers)
    if res.status_code == 200:
        return ds.target_names[np.array(res.json()["predictions"])[0]]
    else:
        print("Failed with ",res.status_code)
        return []
    
def explain(X, name, svc_hostname, cluster_ip):
    formData = {
    'instances': X
    }
    headers = {}
    headers["Host"] = svc_hostname
    res = requests.post('http://'+cluster_ip+'/v1/models/'+name+':explain', json=formData, headers=headers)
    if res.status_code == 200:
        return res.json()
    else:
        print("Failed with ",res.status_code)
        return []

def show_bar(X, labels, title):
    fig = go.Figure(go.Bar(x=X,y=labels,orientation='h',width=[0.5]))
    fig.update_layout(autosize=False,width=700,height=300,
                      xaxis=dict(range=[0, 1]),
                      title_text=title,  
                      font=dict(family="Courier New, monospace",size=18,color="#7f7f7f"
    ))
    fig.show()

    
def show_feature_coverage(exp):
    data = []
    for idx, name in enumerate(exp["names"]):
        data.append(go.Bar(name=name, x=["coverage"], y=[exp['raw']['coverage'][idx]]))
    fig = go.Figure(data=data)
    fig.update_layout(yaxis=dict(range=[0, 1]))
    fig.show()
    
def show_anchors(names):
    display(Markdown('# Explanation:'))
    display(Markdown('## {}'.format(names)))
    
def show_examples(exp,fidx,ds,covered=True):
    if covered:
        cname = 'covered'
        display(Markdown("## Examples covered by Anchors: {}".format(exp['names'][0:fidx+1])))
    else:
        cname = 'covered_false'
        display(Markdown("## Examples not covered by Anchors: {}".format(exp['names'][0:fidx+1])))
    if "feature_names" in ds:
        return pd.DataFrame(exp['raw']['examples'][fidx][cname],columns=ds.feature_names)
    else:
        return pd.DataFrame(exp['raw']['examples'][fidx][cname])

def show_prediction(prediction):
    display(Markdown('## Prediction: {}'.format(prediction)))
    
def show_row(X,ds):
    display(pd.DataFrame(X,columns=ds.feature_names))
                        
