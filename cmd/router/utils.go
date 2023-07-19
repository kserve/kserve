package main

import "fmt"

type InferenceGraphRoutingError struct {
	ErrorMessage string `json:"error"`
	Cause        string `json:"cause"`
}

func (e *InferenceGraphRoutingError) Error() string {
	return fmt.Sprintf("%s. %s", e.ErrorMessage, e.Cause)
}

var (
	RouteNotFoundInSwitchErrMsg = `{"err": "None of the routes matched with the switch condition", "cause":"None of the routes matched with the switch condition with the switch condition under node %v"}`
	ErrorWhenCallingServiceUrl  = `{"err":"Could not call service url %v", "cause":"%v"}`
)
