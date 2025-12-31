// Package utils provides utility functions for the censor system.
package utils

import (
	"fmt"
	"sync"
	"time"
)

// IDGenerator generates unique IDs using a snowflake-like algorithm.
type IDGenerator struct {
	mu        sync.Mutex
	lastTime  int64
	sequence  int64
	machineID int64
}

// NewIDGenerator creates a new ID generator.
func NewIDGenerator() *IDGenerator {
	return NewIDGeneratorWithMachine(0)
}

// NewIDGeneratorWithMachine creates a new ID generator with a specific machine ID.
func NewIDGeneratorWithMachine(machineID int64) *IDGenerator {
	return &IDGenerator{
		machineID: machineID & 0x3FF, // 10 bits for machine ID
	}
}

// Generate generates a unique ID.
func (g *IDGenerator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()

	if now == g.lastTime {
		g.sequence++
		if g.sequence >= 4096 { // 12 bits for sequence
			// Wait for next millisecond
			for now <= g.lastTime {
				time.Sleep(time.Microsecond * 100)
				now = time.Now().UnixMilli()
			}
			g.sequence = 0
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	// Format: timestamp (41 bits) | machine (10 bits) | sequence (12 bits)
	id := (now << 22) | (g.machineID << 12) | g.sequence

	return fmt.Sprintf("%d", id)
}

// GenerateWithPrefix generates a unique ID with a prefix.
func (g *IDGenerator) GenerateWithPrefix(prefix string) string {
	return prefix + "_" + g.Generate()
}
