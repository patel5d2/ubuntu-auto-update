package updater

import (
	"strings"
	"testing"
)

func TestNewUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		id, err := newUUID()
		if err != nil {
			t.Fatalf("newUUID() error: %v", err)
		}
		// Validate format: 8-4-4-4-12
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("newUUID() = %q, want 5 dash-separated groups", id)
			continue
		}
		if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 ||
			len(parts[3]) != 4 || len(parts[4]) != 12 {
			t.Errorf("newUUID() = %q, wrong group lengths", id)
		}
		// Validate version bits
		if parts[2][0] != '4' {
			t.Errorf("newUUID() version nibble = %c, want '4'", parts[2][0])
		}
		// Variant bits: first nibble of group[3] must be 8, 9, a, or b
		v := parts[3][0]
		if v != '8' && v != '9' && v != 'a' && v != 'b' {
			t.Errorf("newUUID() variant nibble = %c, want 8/9/a/b", v)
		}
		if seen[id] {
			t.Errorf("newUUID() returned duplicate: %s", id)
		}
		seen[id] = true
	}
}

func TestPercent_EdgeCases(t *testing.T) {
	if got := percent(0, 0); got != 0 {
		t.Errorf("percent(0,0) = %d, want 0", got)
	}
	if got := percent(3, 3); got != 100 {
		t.Errorf("percent(3,3) = %d, want 100", got)
	}
}

func TestShouldAbort_Boundary(t *testing.T) {
	// Threshold of exactly 100 with 100% observed → abort
	if !shouldAbort(100, 100) {
		t.Error("shouldAbort(100,100) = false, want true")
	}
	// Threshold 0 always false
	for _, obs := range []int{0, 50, 100} {
		if shouldAbort(0, obs) {
			t.Errorf("shouldAbort(0,%d) = true, want false", obs)
		}
	}
	// Threshold 101 (out of range) always false
	if shouldAbort(101, 100) {
		t.Error("shouldAbort(101,100) = true, want false (out of range)")
	}
}

func TestBuildUpdateScript_Root(t *testing.T) {
	s := buildUpdateScript("root")
	if strings.Contains(s, "sudo") {
		t.Error("root user script should not contain sudo")
	}
	if !strings.Contains(s, "apt-get") {
		t.Error("expected apt-get in script")
	}
	if !strings.Contains(s, "pipefail") {
		t.Error("expected pipefail in script")
	}
}

func TestBuildUpdateScript_NonRoot(t *testing.T) {
	s := buildUpdateScript("ubuntu")
	if !strings.Contains(s, "sudo -n") {
		t.Errorf("non-root script should use sudo -n, got: %s", s)
	}
	if !strings.Contains(s, "apt-get") {
		t.Error("expected apt-get in script")
	}
}

func TestBuildUpdateScript_EmptyUser(t *testing.T) {
	// Empty user is treated as non-root (gets sudo)
	s := buildUpdateScript("")
	// Should not panic and should contain apt-get
	if !strings.Contains(s, "apt-get") {
		t.Error("expected apt-get even with empty user")
	}
}

func TestCoordinator_InFlightCount_Empty(t *testing.T) {
	c := &Coordinator{inFlightGroups: make(map[string]struct{})}
	if n := c.InFlightCount(); n != 0 {
		t.Errorf("InFlightCount() = %d, want 0", n)
	}
}

func TestCoordinator_InFlightCount_AfterAdd(t *testing.T) {
	c := &Coordinator{inFlightGroups: make(map[string]struct{})}
	c.inFlightGroups["group1"] = struct{}{}
	c.inFlightGroups["group2"] = struct{}{}
	if n := c.InFlightCount(); n != 2 {
		t.Errorf("InFlightCount() = %d, want 2", n)
	}
}

func TestStart_EmptyHostIDs(t *testing.T) {
	c := &Coordinator{inFlightGroups: make(map[string]struct{})}
	_, err := c.Start(nil, BulkRunOptions{HostIDs: []int32{}})
	if err == nil || !strings.Contains(err.Error(), "no hosts selected") {
		t.Errorf("expected 'no hosts selected' error, got %v", err)
	}
}

func TestConcurrencyClamp(t *testing.T) {
	// Test that concurrency clamping logic works via BulkRunOptions
	// We can't easily test Start() without a real DB, but we can verify
	// the clamping constants are sane
	if DefaultConcurrency <= 0 {
		t.Error("DefaultConcurrency must be positive")
	}
	if MaxConcurrency < DefaultConcurrency {
		t.Error("MaxConcurrency must be >= DefaultConcurrency")
	}
}

func TestSkipRemaining_EmptySlice(t *testing.T) {
	c := &Coordinator{inFlightGroups: make(map[string]struct{})}
	// Should not panic
	c.skipRemaining([]int32{}, []int32{}, "test reason")
}
