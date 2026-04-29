package updater

import "testing"

func TestPercent(t *testing.T) {
	cases := []struct {
		part, total, want int
	}{
		{0, 0, 0},
		{0, 5, 0},
		{1, 4, 25},
		{2, 4, 50},
		{3, 4, 75},
		{5, 5, 100},
	}
	for _, c := range cases {
		if got := percent(c.part, c.total); got != c.want {
			t.Errorf("percent(%d,%d) = %d, want %d", c.part, c.total, got, c.want)
		}
	}
}

func TestShouldAbort(t *testing.T) {
	cases := []struct {
		threshold, observed int
		want                bool
	}{
		{0, 100, false},   // disabled
		{-10, 100, false}, // disabled
		{50, 49, false},
		{50, 50, true},
		{50, 100, true},
		{100, 100, true},
		{101, 100, false}, // out of range disables
	}
	for _, c := range cases {
		if got := shouldAbort(c.threshold, c.observed); got != c.want {
			t.Errorf("shouldAbort(t=%d,o=%d) = %v, want %v", c.threshold, c.observed, got, c.want)
		}
	}
}
