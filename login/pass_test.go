package login

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestParseFingerprint(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name: "valid fpr line",
			input: `pub:-:2048:1:AABBCCDD:...
fpr:::::::::ABCDEF1234567890ABCDEF1234567890ABCDEF12:
uid:...`,
			want: "ABCDEF1234567890ABCDEF1234567890ABCDEF12",
			ok:   true,
		},
		{
			name:  "no fpr line",
			input: "pub:-:2048:1:AABBCCDD\nuid:foo\n",
			want:  "",
			ok:    false,
		},
		{
			name:  "fpr with empty fingerprint field",
			input: "fpr:::::::::\n",
			want:  "",
			ok:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseFingerprint(tc.input)
			assert.Check(t, cmp.Equal(ok, tc.ok))
			assert.Check(t, cmp.Equal(got, tc.want))
		})
	}
}
