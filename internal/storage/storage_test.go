package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RecordAndRetrieve(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.db")
	s, err := New(f)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	err = s.Record(UsageRecord{
		Timestamp:    time.Now(),
		Model:        "gpt-4o",
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
		Stream:       false,
		Duration:     100,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	records := s.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", records[0].Model)
	}
	if records[0].TotalTokens != 30 {
		t.Errorf("expected total 30, got %d", records[0].TotalTokens)
	}
}

func TestStore_Persistence(t *testing.T) {
	f := filepath.Join(t.TempDir(), "persist.db")

	s1, _ := New(f)
	s1.Record(UsageRecord{
		Timestamp:    time.Now().Add(-time.Hour),
		Model:        "deepseek-v4-pro",
		InputTokens:  5,
		OutputTokens: 15,
		TotalTokens:  20,
		Stream:       true,
		Duration:     200,
	})
	s1.Record(UsageRecord{
		Timestamp:    time.Now(),
		Model:        "claude-sonnet-4.6",
		InputTokens:  50,
		OutputTokens: 100,
		TotalTokens:  150,
		Stream:       false,
		Duration:     500,
	})
	s1.Close()

	s2, err := New(f)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()

	records := s2.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records after reopen, got %d", len(records))
	}
	if records[0].Model != "deepseek-v4-pro" {
		t.Errorf("expected first model deepseek-v4-pro, got %s", records[0].Model)
	}
	if records[1].Model != "claude-sonnet-4.6" {
		t.Errorf("expected second model claude-sonnet-4.6, got %s", records[1].Model)
	}
}

func TestStore_Since(t *testing.T) {
	f := filepath.Join(t.TempDir(), "since.db")
	s, _ := New(f)
	defer s.Close()

	now := time.Now()
	s.Record(UsageRecord{Timestamp: now.Add(-2 * time.Hour), Model: "old", TotalTokens: 10})
	s.Record(UsageRecord{Timestamp: now.Add(-30 * time.Minute), Model: "recent", TotalTokens: 20})
	s.Record(UsageRecord{Timestamp: now, Model: "now", TotalTokens: 30})

	records := s.Since(now.Add(-1 * time.Hour))
	if len(records) != 2 {
		t.Fatalf("expected 2 records since 1h ago, got %d", len(records))
	}
	if records[0].Model != "recent" {
		t.Errorf("expected first=recent, got %s", records[0].Model)
	}
}

func TestStore_Between(t *testing.T) {
	f := filepath.Join(t.TempDir(), "between.db")
	s, _ := New(f)
	defer s.Close()

	now := time.Now()
	s.Record(UsageRecord{Timestamp: now.Add(-3 * time.Hour), Model: "too-old"})
	s.Record(UsageRecord{Timestamp: now.Add(-90 * time.Minute), Model: "in-range"})
	s.Record(UsageRecord{Timestamp: now, Model: "too-new"})

	records := s.Between(now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	if len(records) != 1 {
		t.Fatalf("expected 1 record in range, got %d", len(records))
	}
	if records[0].Model != "in-range" {
		t.Errorf("expected in-range, got %s", records[0].Model)
	}
}

func TestStore_Count(t *testing.T) {
	f := filepath.Join(t.TempDir(), "count.db")
	s, _ := New(f)
	defer s.Close()

	if s.Count() != 0 {
		t.Errorf("expected 0, got %d", s.Count())
	}

	s.Record(UsageRecord{Timestamp: time.Now(), Model: "a"})
	s.Record(UsageRecord{Timestamp: time.Now(), Model: "b"})

	if s.Count() != 2 {
		t.Errorf("expected 2, got %d", s.Count())
	}
}

func TestStore_EmptyDB(t *testing.T) {
	f := filepath.Join(t.TempDir(), "empty.db")
	s, err := New(f)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if len(s.Records()) != 0 {
		t.Errorf("expected 0 records")
	}
}
