from alibiexplainer.explainer_wrapper import ExplainerWrapper

def test_merge_args():
    args = {"threshold": 0.9, "tau": 0.1}
    requestArgs = {"tau":0.4}
    argsNew =  ExplainerWrapper.mergeArgs(args, requestArgs)
    assert argsNew["tau"] == 0.4
    assert argsNew["threshold"] == 0.9