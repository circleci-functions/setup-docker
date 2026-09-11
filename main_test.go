package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGossFlagProvided(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"bare flag", []string{"--install-goss"}, true},
		{"explicit true", []string{"--install-goss=true"}, true},
		{"explicit false", []string{"--install-goss=false"}, true},
		{"absent", []string{"--install-docker-compose=false"}, false},
		{"none", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, gossFlagProvided(tc.args), tc.want)
		})
	}
}
