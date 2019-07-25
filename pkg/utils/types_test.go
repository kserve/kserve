package utils

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestBool(t *testing.T) {
	input := true
	expected := &input
	result := Bool(input)
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}
}

func TestUInt64(t *testing.T) {
	input := uint64(63)
	expected := &input
	result := UInt64(input)
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}
}
