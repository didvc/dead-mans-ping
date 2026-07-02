package main

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		bad  bool
	}{
		{"3d", 72 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"30s", 30 * time.Second, false},
		{"1d12h", 36 * time.Hour, false},
		{"1.5d", 36 * time.Hour, false},
		{"", 0, true},
		{"3x", 0, true},
	}
	for _, c := range cases {
		got, err := parseDuration("--test", c.in)
		if c.bad {
			if err == nil {
				t.Errorf("parseDuration(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
