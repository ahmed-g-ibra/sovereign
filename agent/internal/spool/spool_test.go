package spool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sovereign/agent/internal/probe"
)

func results(n int, prefix string) []probe.Result {
	out := make([]probe.Result, n)
	for i := range out {
		out[i] = probe.Result{
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
			TargetID:  prefix,
			Provider:  "aws",
			TotalMs:   float64(10 + i),
		}
	}
	return out
}

func newSpool(t *testing.T) (*Spool[probe.Result], string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New[probe.Result](dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestAppendClaimCommit(t *testing.T) {
	s, _ := newSpool(t)
	if err := s.Append(results(3, "aws-me-central-1")); err != nil {
		t.Fatal(err)
	}

	got, commit, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("claimed %d results, want 3", len(got))
	}
	if got[0].TargetID != "aws-me-central-1" || got[2].TotalMs != 12 {
		t.Errorf("results did not round-trip: %+v", got)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}

	after, _, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("committed data came back: %d results", len(after))
	}
}

func TestFailedUploadKeepsData(t *testing.T) {
	// The reason this package exists: an upload that fails must lose nothing.
	s, _ := newSpool(t)
	if err := s.Append(results(5, "gcp-me-central1")); err != nil {
		t.Fatal(err)
	}

	first, _, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a failed upload: never call commit.

	second, commit, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("retry claimed %d results, want %d", len(second), len(first))
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
}

func TestDataAppendedDuringSendIsNotLost(t *testing.T) {
	s, _ := newSpool(t)
	if err := s.Append(results(2, "first")); err != nil {
		t.Fatal(err)
	}

	claimed, commit, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d, want 2", len(claimed))
	}

	// A measurement round finishes while the upload is still in flight.
	if err := s.Append(results(3, "second")); err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}

	next, _, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 3 {
		t.Fatalf("claimed %d after commit, want the 3 appended mid-send", len(next))
	}
	if next[0].TargetID != "second" {
		t.Errorf("wrong data survived: %q", next[0].TargetID)
	}
}

func TestSurvivesRestart(t *testing.T) {
	s, dir := newSpool(t)
	if err := s.Append(results(4, "persisted")); err != nil {
		t.Fatal(err)
	}

	// A new process, same directory.
	reopened, err := New[probe.Result](dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := reopened.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("after restart claimed %d results, want 4", len(got))
	}
}

func TestUnclaimedSendFileIsRecoveredAfterRestart(t *testing.T) {
	s, dir := newSpool(t)
	if err := s.Append(results(2, "interrupted")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Claim(); err != nil { // crash before commit
		t.Fatal(err)
	}

	reopened, err := New[probe.Result](dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := reopened.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("in-flight data lost across restart: got %d, want 2", len(got))
	}
}

func TestSizeCapDropsOldestNotNewest(t *testing.T) {
	dir := t.TempDir()
	s, err := New[probe.Result](dir, 2048) // deliberately tiny
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(results(50, "old")); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(results(50, "new")); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || len(got) >= 100 {
		t.Fatalf("expected the spool to be trimmed, got %d results", len(got))
	}
	if got[len(got)-1].TargetID != "new" {
		t.Errorf("newest data was dropped; last entry is %q", got[len(got)-1].TargetID)
	}
}

func TestCorruptLineDoesNotStrandTheRest(t *testing.T) {
	s, dir := newSpool(t)
	if err := s.Append(results(2, "good")); err != nil {
		t.Fatal(err)
	}
	// Simulate a power cut mid-write.
	f, err := os.OpenFile(filepath.Join(dir, activeName), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{\"ts\":\"not-a-timestamp\"\n")
	f.Close()

	if err := s.Append(results(1, "after")); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3 readable ones around the corrupt line", len(got))
	}
}

func TestPendingReportsBytes(t *testing.T) {
	s, _ := newSpool(t)
	if s.Pending() != 0 {
		t.Errorf("empty spool reports %d bytes", s.Pending())
	}
	if err := s.Append(results(3, "x")); err != nil {
		t.Fatal(err)
	}
	if s.Pending() == 0 {
		t.Error("spool with data reports 0 bytes pending")
	}
}

// A second payload type shares the implementation: the agent spools latency
// samples every minute and traceroutes every hour, with identical semantics
// and separate files.
type tracePayload struct {
	TargetID string `json:"target_id"`
	Hops     int    `json:"hops"`
}

func TestSpoolIsGenericOverPayload(t *testing.T) {
	s, _ := func() (*Spool[tracePayload], string) {
		dir := t.TempDir()
		sp, err := New[tracePayload](dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		return sp, dir
	}()

	if err := s.Append([]tracePayload{{TargetID: "aws-me-central-1", Hops: 10}}); err != nil {
		t.Fatal(err)
	}
	got, commit, err := s.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Hops != 10 || got[0].TargetID != "aws-me-central-1" {
		t.Fatalf("payload did not round-trip: %+v", got)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
}
