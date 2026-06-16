package naming

import "testing"

func TestToolchainSmoke(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("math is broken")
	}
}
