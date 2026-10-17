package main

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestSplitDockerfiles(t *testing.T) {
	cases := []struct {
		name  string
		input string
		sep   rune
		want  []string
	}{
		{name: "single", input: "Dockerfile", want: []string{"Dockerfile"}},
		{name: "colon separated", input: "a/Dockerfile:b/Dockerfile", want: []string{"a/Dockerfile", "b/Dockerfile"}},
		{name: "newline separated", input: "a/Dockerfile\nb/Dockerfile", want: []string{"a/Dockerfile", "b/Dockerfile"}},
		{name: "mixed", input: "a/Dockerfile:b/Dockerfile\nc/Dockerfile", want: []string{"a/Dockerfile", "b/Dockerfile", "c/Dockerfile"}},
		{name: "trims spaces", input: " a/Dockerfile : b/Dockerfile ", want: []string{"a/Dockerfile", "b/Dockerfile"}},
		{name: "drops empties", input: "a/Dockerfile::b/Dockerfile", want: []string{"a/Dockerfile", "b/Dockerfile"}},
		{name: "empty string", input: "", want: nil},
		{name: "windows drive paths", input: `C:\a\Dockerfile;D:\b\Dockerfile`, sep: ';',
			want: []string{`C:\a\Dockerfile`, `D:\b\Dockerfile`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sep := tc.sep
			if sep == 0 {
				sep = ':'
			}
			got := splitDockerfiles(tc.input, sep)
			assert.Check(t, cmp.DeepEqual(got, tc.want))
		})
	}
}

func TestHadolintArgs(t *testing.T) {
	cases := []struct {
		name      string
		threshold string
		ignore    string
		trusted   string
		format    string
		file      string
		want      []string
	}{
		{
			name:      "basic",
			threshold: "info",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "info", "Dockerfile"},
		},
		{
			name:      "threshold none passes through",
			threshold: "none",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "none", "Dockerfile"},
		},
		{
			name:      "with format",
			threshold: "error",
			format:    "junit",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "error", "--format", "junit", "Dockerfile"},
		},
		{
			name:      "ignore rules",
			threshold: "warning",
			ignore:    "DL3008,SC2086",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "warning", "--ignore", "DL3008", "--ignore", "SC2086", "Dockerfile"},
		},
		{
			name:      "trusted registries",
			threshold: "info",
			trusted:   "docker.io,gcr.io",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "info", "--trusted-registry", "docker.io", "--trusted-registry", "gcr.io", "Dockerfile"},
		},
		{
			name:      "ignore with spaces",
			threshold: "info",
			ignore:    " DL3008 , SC2086 ",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "info", "--ignore", "DL3008", "--ignore", "SC2086", "Dockerfile"},
		},
		{
			name:      "all flags",
			threshold: "error",
			ignore:    "DL3008",
			trusted:   "docker.io",
			format:    "junit",
			file:      "Dockerfile",
			want:      []string{"--failure-threshold", "error", "--format", "junit", "--ignore", "DL3008", "--trusted-registry", "docker.io", "Dockerfile"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hadolintArgs(tc.threshold, tc.ignore, tc.trusted, tc.format, tc.file)
			assert.Check(t, cmp.DeepEqual(got, tc.want))
		})
	}
}
