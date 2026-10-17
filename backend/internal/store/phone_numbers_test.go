package store

import (
	"testing"
)

func TestNormalizeE164(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"+55 11 99999-8888", "+5511999998888"},
		{"5511999998888", "+5511999998888"},
		{"+1 (800) 555-0199", "+18005550199"},
		{"  +55 (21) 98765-4321  ", "+5521987654321"},
		{"", ""},
	}

	for _, c := range cases {
		got := normalizeE164(c.input)
		if got != c.expected {
			t.Errorf("normalizeE164(%q) = %q; expected %q", c.input, got, c.expected)
		}
	}
}
