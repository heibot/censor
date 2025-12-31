package utils

import (
	"sync"
	"testing"
)

func TestIDGenerator_Generate(t *testing.T) {
	gen := NewIDGenerator()

	id := gen.Generate()

	if id == "" {
		t.Error("Generate() returned empty string")
	}

	// Should be numeric
	for _, c := range id {
		if c < '0' || c > '9' {
			t.Errorf("Generate() returned non-numeric ID: %s", id)
			break
		}
	}
}

func TestIDGenerator_Unique(t *testing.T) {
	gen := NewIDGenerator()

	ids := make(map[string]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id := gen.Generate()
		if ids[id] {
			t.Fatalf("Generate() produced duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestIDGenerator_Concurrent(t *testing.T) {
	gen := NewIDGenerator()

	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make(map[string]bool)
	goroutines := 10
	idsPerGoroutine := 1000

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localIDs := make([]string, idsPerGoroutine)

			for j := 0; j < idsPerGoroutine; j++ {
				localIDs[j] = gen.Generate()
			}

			mu.Lock()
			for _, id := range localIDs {
				if ids[id] {
					t.Errorf("Concurrent Generate() produced duplicate ID: %s", id)
				}
				ids[id] = true
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	expectedCount := goroutines * idsPerGoroutine
	if len(ids) != expectedCount {
		t.Errorf("Generated %d unique IDs, want %d", len(ids), expectedCount)
	}
}

func TestIDGenerator_WithMachine(t *testing.T) {
	gen1 := NewIDGeneratorWithMachine(1)
	gen2 := NewIDGeneratorWithMachine(2)

	id1 := gen1.Generate()
	id2 := gen2.Generate()

	if id1 == id2 {
		t.Error("Different machines generated same ID")
	}
}

func TestIDGenerator_GenerateWithPrefix(t *testing.T) {
	gen := NewIDGenerator()

	prefix := "biz"
	id := gen.GenerateWithPrefix(prefix)

	if len(id) <= len(prefix)+1 {
		t.Errorf("GenerateWithPrefix() returned too short ID: %s", id)
	}

	expectedPrefix := prefix + "_"
	if id[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("GenerateWithPrefix() ID doesn't start with %q: %s", expectedPrefix, id)
	}
}

func BenchmarkIDGenerator_Generate(b *testing.B) {
	gen := NewIDGenerator()

	for i := 0; i < b.N; i++ {
		gen.Generate()
	}
}

func BenchmarkIDGenerator_Concurrent(b *testing.B) {
	gen := NewIDGenerator()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gen.Generate()
		}
	})
}
