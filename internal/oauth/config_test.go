package oauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "host only",
			input:    "survey.openmeet.net",
			expected: "survey.openmeet.net",
		},
		{
			name:     "with https prefix",
			input:    "https://survey.openmeet.net",
			expected: "survey.openmeet.net",
		},
		{
			name:     "with http prefix",
			input:    "http://survey.openmeet.net",
			expected: "survey.openmeet.net",
		},
		{
			name:     "with trailing slash",
			input:    "survey.openmeet.net/",
			expected: "survey.openmeet.net",
		},
		{
			name:     "with https and trailing slash",
			input:    "https://survey.openmeet.net/",
			expected: "survey.openmeet.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHost(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
