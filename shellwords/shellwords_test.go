package shellwords

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "Empty", input: "", want: nil},
		{name: "Simple", input: "a b c", want: []string{"a", "b", "c"}},
		{name: "SingleQuotes", input: `'foo bar'`, want: []string{"foo bar"}},
		{name: "DoubleQuotes", input: `"foo bar"`, want: []string{"foo bar"}},
		{name: "BackslashEscape", input: `foo\ bar`, want: []string{"foo bar"}},
		{name: "Mixed", input: `--label "foo=bar baz" --label 'other=x'`, want: []string{"--label", "foo=bar baz", "--label", "other=x"}},
		{name: "UnterminatedSingle", input: `'abc`, wantErr: "unterminated single quote"},
		{name: "UnterminatedDouble", input: `"abc`, wantErr: "unterminated double quote"},
		{name: "TrailingBackslash", input: `abc\`, wantErr: "trailing backslash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.input)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tt.want))
		})
	}
}
