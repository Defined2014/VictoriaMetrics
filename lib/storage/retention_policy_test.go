package storage

import (
	"testing"
	"time"
)

func TestParseRetentionRuleSuccess(t *testing.T) {
	r, err := parseRetentionRule("account=1,project!=999:7d", (30 * 24 * time.Hour).Milliseconds())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got, want := len(r.matchers), 2; got != want {
		t.Fatalf("unexpected matchers count; got %d; want %d", got, want)
	}
	if got, want := r.retentionMsecs, (7 * 24 * time.Hour).Milliseconds(); got != want {
		t.Fatalf("unexpected retention msecs; got %d; want %d", got, want)
	}
}

func TestParseRetentionRuleFailure(t *testing.T) {
	testCases := []string{
		"",
		"account=1",
		"account=1:",
		":7d",
		"team=1:7d",
		"account=foo:7d",
	}
	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			if _, err := parseRetentionRule(tc, (30 * 24 * time.Hour).Milliseconds()); err == nil {
				t.Fatalf("expecting error for %q", tc)
			}
		})
	}
}

func TestParseRetentionRuleExceedsGlobalRetention(t *testing.T) {
	_, err := parseRetentionRule("account=1:31d", (30 * 24 * time.Hour).Milliseconds())
	if err == nil {
		t.Fatalf("expecting non-nil error")
	}
}

func TestRetentionPolicyMatchAndFallback(t *testing.T) {
	p, err := newRetentionPolicy(30*24*time.Hour, []string{
		"account=1,project!=999:7d",
		"project=999:20d",
		"account=1:10d",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	f := func(accountID, projectID uint32, want time.Duration) {
		t.Helper()
		got := p.retentionMsecsForTenant(accountID, projectID)
		if got != want.Milliseconds() {
			t.Fatalf("unexpected retention for %d:%d; got %dms; want %dms", accountID, projectID, got, want.Milliseconds())
		}
	}

	f(1, 1, 7*24*time.Hour)
	f(1, 999, 10*24*time.Hour)
	f(2, 999, 20*24*time.Hour)
	f(2, 2, 30*24*time.Hour)
}

func TestRetentionPolicyDeadlineForTenant(t *testing.T) {
	p, err := newRetentionPolicy(30*24*time.Hour, []string{"account=1:7d"})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	s := &Storage{
		retentionMsecs: 30 * 24 * int64(time.Hour/time.Millisecond),
	}
	s.retentionPolicy.Store(p)
	nowMsecs := int64((40 * 24 * time.Hour) / time.Millisecond)
	if got := s.retentionDeadlineForTenant(nowMsecs, 1, 1); got != nowMsecs-(7*24*time.Hour).Milliseconds() {
		t.Fatalf("unexpected deadline for account 1")
	}
	if got := s.retentionDeadlineForTenant(nowMsecs, 2, 1); got != nowMsecs-(30*24*time.Hour).Milliseconds() {
		t.Fatalf("unexpected deadline for account 2")
	}
}
