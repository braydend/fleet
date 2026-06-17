package selfupdate

import "testing"

func TestIsDev(t *testing.T) {
	for _, v := range []string{"dev", "", "garbage", "v"} {
		if !IsDev(v) {
			t.Errorf("IsDev(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0.1.0", "v0.1.0", "1.2.3"} {
		if IsDev(v) {
			t.Errorf("IsDev(%q) = true, want false", v)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "0.2.0", true},
		{"v0.1.0", "v0.1.1", true},
		{"0.1.0", "1.0.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.2.0", "0.1.9", false},
		{"dev", "0.2.0", false},
		{"0.1.0", "garbage", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q,%q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
