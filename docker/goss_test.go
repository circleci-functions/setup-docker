package docker

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGossSupport(t *testing.T) {
	tests := []struct {
		name     string
		linux    bool
		explicit bool
		wantSkip bool
		wantErr  bool
	}{
		{"linux defaulted", true, false, false, false},
		{"linux explicit", true, true, false, false},
		{"non-linux defaulted", false, false, true, false},
		{"non-linux explicit", false, true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			skip, err := gossSupport(tc.linux, tc.explicit)
			assert.Equal(t, skip, tc.wantSkip)
			assert.Equal(t, err != nil, tc.wantErr)
		})
	}
}
