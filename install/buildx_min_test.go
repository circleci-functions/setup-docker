package install

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestOlderThan(t *testing.T) {
	tests := []struct {
		name string
		have string
		want bool
	}{
		{name: "missing", have: "", want: true},
		{name: "older", have: "0.9.1", want: true},
		{name: "equal", have: "0.15.0", want: false},
		{name: "newer", have: "0.37.1", want: false},
		{name: "leading v", have: "v0.14.9", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Check(t, cmp.Equal(olderThan(tt.have, "0.15.0"), tt.want))
		})
	}
}
