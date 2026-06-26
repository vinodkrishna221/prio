package agent

import (
	"testing"
)

func TestGetClientSingleton(t *testing.T) {
	c1, err1 := GetClient()
	c2, err2 := GetClient()

	if err1 != err2 {
		t.Errorf("expected same error, got %v and %v", err1, err2)
	}

	if c1 != c2 {
		t.Error("expected GetClient to return the same singleton instance")
	}
}
