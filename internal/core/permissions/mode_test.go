package permissions

import "testing"

func TestNextSkipsAlwaysApproveWithoutOptIn(t *testing.T) {
	m := Ask
	seen := map[Mode]bool{}
	for range 10 {
		m = m.Next(false)
		seen[m] = true
	}
	if seen[AlwaysApprove] {
		t.Fatal("always-approve reached without opt-in")
	}
	if !seen[Ask] || !seen[AutoEdit] || !seen[Plan] {
		t.Fatalf("cycle incomplete: %v", seen)
	}
}

func TestNextWithOptIn(t *testing.T) {
	if got := Plan.Next(true); got != AlwaysApprove {
		t.Fatalf("Plan.Next(true) = %v", got)
	}
	if got := AlwaysApprove.Next(true); got != Ask {
		t.Fatalf("AlwaysApprove.Next(true) = %v", got)
	}
	// Opt-in revoked while in always-approve: fall back to the safe default.
	if got := AlwaysApprove.Next(false); got != Ask {
		t.Fatalf("AlwaysApprove.Next(false) = %v", got)
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, m := range []Mode{Ask, AutoEdit, Plan, AlwaysApprove} {
		got, err := Parse(m.String())
		if err != nil || got != m {
			t.Fatalf("Parse(%q) = %v, %v", m, got, err)
		}
	}
	if _, err := Parse("yolo"); err == nil {
		t.Fatal("expected error")
	}
}
