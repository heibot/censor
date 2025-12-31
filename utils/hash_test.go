package utils

import (
	"testing"
)

func TestHashText(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "simple text",
			input: "hello world",
		},
		{
			name:  "unicode text",
			input: "你好世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashText(tt.input)

			// SHA256 produces 64 hex characters
			if len(result) != 64 {
				t.Errorf("HashText() produced hash of length %d, want 64", len(result))
			}

			// Same input should produce same hash
			result2 := HashText(tt.input)
			if result != result2 {
				t.Error("HashText() is not deterministic")
			}
		})
	}
}

func TestHashText_Different(t *testing.T) {
	hash1 := HashText("hello")
	hash2 := HashText("world")

	if hash1 == hash2 {
		t.Error("Different inputs produced same hash")
	}
}

func TestHashURL(t *testing.T) {
	url := "https://example.com/image.jpg"
	result := HashURL(url)

	if len(result) != 64 {
		t.Errorf("HashURL() produced hash of length %d, want 64", len(result))
	}

	// Same URL should produce same hash
	result2 := HashURL(url)
	if result != result2 {
		t.Error("HashURL() is not deterministic")
	}
}

func TestQuickHash(t *testing.T) {
	data := "test data"
	result := QuickHash(data)

	// Should be non-zero
	if result == 0 {
		t.Error("QuickHash() returned 0")
	}

	// Same input should produce same hash
	result2 := QuickHash(data)
	if result != result2 {
		t.Error("QuickHash() is not deterministic")
	}

	// Different input should produce different hash
	result3 := QuickHash("different data")
	if result == result3 {
		t.Error("Different inputs produced same hash")
	}
}

func TestTruncateHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		length   int
		expected string
	}{
		{
			name:     "shorter than length",
			hash:     "abc",
			length:   10,
			expected: "abc",
		},
		{
			name:     "equal to length",
			hash:     "abcde",
			length:   5,
			expected: "abcde",
		},
		{
			name:     "longer than length",
			hash:     "abcdefghij",
			length:   5,
			expected: "abcde",
		},
		{
			name:     "length zero",
			hash:     "abc",
			length:   0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateHash(tt.hash, tt.length)
			if result != tt.expected {
				t.Errorf("TruncateHash(%q, %d) = %q, want %q", tt.hash, tt.length, result, tt.expected)
			}
		})
	}
}

func BenchmarkHashText(b *testing.B) {
	text := "This is a sample text for benchmarking the hash function."
	for i := 0; i < b.N; i++ {
		HashText(text)
	}
}

func BenchmarkQuickHash(b *testing.B) {
	text := "This is a sample text for benchmarking the hash function."
	for i := 0; i < b.N; i++ {
		QuickHash(text)
	}
}
