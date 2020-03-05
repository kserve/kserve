package constants

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestVirtualServiceHostname(t *testing.T) {

	expected := "kftest.user1.example.com"
	result := VirtualServiceHostname("kftest-predictor-default.user1.example.com")
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}

	expected = "kftest-user1.example.com"
	result = VirtualServiceHostname("kftest-predictor-default-user1.example.com")
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}
}
