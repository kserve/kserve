


#main.py

import pandas as pd
from statsmodels.tsa.holtwinters import ExponentialSmoothing
import numpy as np
import joblib
from holt import holt

df=pd.read_csv('https://raw.githubusercontent.com/jbrownlee/Datasets/master/shampoo.csv')

#Taking a test-train split of 80 %
train=df[0:int(len(df)*0.8)] 
test=df[int(len(df)*0.8):]

#Pre-processing the  Month  field
train.Timestamp = pd.to_datetime(train.Month,format='%m-%d') 
train.index = train.Timestamp 
test.Timestamp = pd.to_datetime(test.Month,format='%m-%d') 
test.index = test.Timestamp 

#fitting the model based on  optimal parameters
model = ExponentialSmoothing(np.asarray(train['Sales']) ,seasonal_periods=7 ,trend='add', seasonal='add',).fit()
joblib.dump(model,'model_holt.sav')
