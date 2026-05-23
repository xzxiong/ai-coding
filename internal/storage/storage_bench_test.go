package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkStore_Record(b *testing.B) {
	f := filepath.Join(b.TempDir(), "bench.db")
	s, err := New(f)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	r := UsageRecord{
		Timestamp:    time.Now(),
		Model:        "deepseek-v4-pro",
		InputTokens:  100,
		OutputTokens: 200,
		TotalTokens:  300,
		Stream:       false,
		Duration:     1500,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Timestamp = time.Now()
		s.Record(r)
	}
}

func BenchmarkStore_Records_100(b *testing.B) {
	benchReadN(b, 100)
}

func BenchmarkStore_Records_1000(b *testing.B) {
	benchReadN(b, 1000)
}

func BenchmarkStore_Records_10000(b *testing.B) {
	benchReadN(b, 10000)
}

func BenchmarkStore_Since_10000(b *testing.B) {
	f := filepath.Join(b.TempDir(), "bench_since.db")
	s, _ := New(f)
	defer s.Close()

	now := time.Now()
	for i := 0; i < 10000; i++ {
		s.Record(UsageRecord{
			Timestamp:    now.Add(-time.Duration(10000-i) * time.Second),
			Model:        "test",
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		})
	}

	since := now.Add(-1 * time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Since(since)
	}
}

func benchReadN(b *testing.B, n int) {
	f := filepath.Join(b.TempDir(), "bench_read.db")
	s, _ := New(f)
	defer s.Close()

	now := time.Now()
	for i := 0; i < n; i++ {
		s.Record(UsageRecord{
			Timestamp:    now.Add(time.Duration(i) * time.Millisecond),
			Model:        "test",
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Records()
	}
}
