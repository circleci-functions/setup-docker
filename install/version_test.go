package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestInstalledVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	cases := []struct {
		name   string
		script string
		args   []string
		want   string
		wantOK bool
	}{
		{
			name: "docker version output",
			script: `#!/bin/sh
echo "Docker version 29.9.1, build abc123"`,
			args:   []string{"--version"},
			want:   "29.9.1",
			wantOK: true,
		},
		{
			name: "goss version output",
			script: `#!/bin/sh
echo "goss version v0.4.10"`,
			args:   []string{"version"},
			want:   "0.4.10",
			wantOK: true,
		},
		{
			name: "bare version",
			script: `#!/bin/sh
echo "v0.15.2"`,
			want:   "0.15.2",
			wantOK: true,
		},
		{
			name: "no version in output",
			script: `#!/bin/sh
echo "no version here"`,
			want:   "",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "fakecmd")
			err := os.WriteFile(bin, []byte(tc.script), 0o755)
			assert.NilError(t, err)

			origPath := os.Getenv("PATH")
			t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

			got, ok := installedVersion(context.Background(), "fakecmd", tc.args...)
			assert.Check(t, cmp.Equal(ok, tc.wantOK))
			assert.Check(t, cmp.Equal(got, tc.want))
		})
	}
}
