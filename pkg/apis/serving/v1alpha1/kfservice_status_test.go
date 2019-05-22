package v1alpha1

import (
	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"k8s.io/api/core/v1"
	"testing"
)

func TestKFServiceDuckType(t *testing.T) {
	tests := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1alpha1.Conditions{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&KFService{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(KFService, %T) = %v", test.t, err)
			}
		})
	}
}

func TestConfigurationIsReady(t *testing.T) {
	cases := []struct {
		name    string
		status  KFServiceStatus
		isReady bool
	}{{
		name:    "empty status should not be ready",
		status:  KFServiceStatus{},
		isReady: false,
	}, {
		name: "Different condition type should not be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   ServiceConditionDefaultConfigurationsReady,
					Status: v1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   ServiceConditionDefaultConfigurationsReady,
					Status: v1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type: ServiceConditionDefaultConfigurationsReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   ServiceConditionDefaultConfigurationsReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status should be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}, {
					Type:   ServiceConditionDefaultConfigurationsReady,
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		status: KFServiceStatus{
			Status: duckv1alpha1.Status{
				Conditions: duckv1alpha1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}, {
					Type:   ServiceConditionDefaultConfigurationsReady,
					Status: v1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		if e, a := tc.isReady, tc.status.IsReady(); e != a {
			t.Errorf("%q expected: %v got: %v", tc.name, e, a)
		}
	}
}
