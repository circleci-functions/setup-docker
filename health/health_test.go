package health

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestRunArgs(t *testing.T) {
	t.Setenv("API_KEY", "s3cret")
	cases := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "http publishes no ports",
			cfg:  Config{Image: "app:t", Port: 8080, Path: "/healthz", Env: "A=1\n\nB=two words\n", PassEnv: "API_KEY"},
			want: []string{"run", "-d", "-e", "A=1", "-e", "B=two words", "-e", "API_KEY", "app:t"},
		},
		{
			name: "HEALTHCHECK polls every 2s",
			cfg:  Config{Image: "app:t", RunArgs: `--network host --label "a=b c"`, Command: `serve --port 80`},
			want: []string{"run", "-d", "--health-interval", "2s", "--network", "host", "--label", "a=b c", "app:t", "serve", "--port", "80"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runArgs(tc.cfg)
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tc.want))
		})
	}
}

func TestRunArgsErrors(t *testing.T) {
	_, err := runArgs(Config{Image: "app:t", Env: "NOVALUE"})
	assert.Check(t, cmp.ErrorContains(err, `env: "NOVALUE" is not KEY=VALUE`))
	_, err = runArgs(Config{Image: "app:t", PassEnv: "SURELY_NOT_SET_42"}) //nolint:gosec // an env var name, not a credential
	assert.Check(t, cmp.ErrorContains(err, "pass-env: SURELY_NOT_SET_42 is not set"))
	_, err = runArgs(Config{Image: "app:t", RunArgs: `"open`})
	assert.Check(t, cmp.ErrorContains(err, "run-args: unterminated double quote"))
}

func TestParseWget(t *testing.T) {
	exit1 := errors.New("exit status 1")
	cases := []struct {
		name    string
		out     string
		err     error
		want    int
		wantErr string
	}{
		{name: "ok", out: "  HTTP/1.1 200 OK\n  Content-Length: 3\n", want: 200},
		{name: "redirect then ok", out: "  HTTP/1.1 302 Found\n  HTTP/1.1 204 No Content\n", want: 204},
		{name: "not found", out: "  HTTP/1.1 404 Not Found\nwget: server returned error: HTTP/1.1 404 Not Found\n", err: exit1, want: 404},
		{name: "refused", out: "wget: can't connect to remote host (127.0.0.1): Connection refused\n", err: exit1, wantErr: "Connection refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseWget(tc.out, tc.err)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(got, tc.want))
		})
	}
}
