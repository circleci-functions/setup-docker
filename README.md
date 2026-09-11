# setup-docker

A CircleCI function that installs the Docker CLI and related tooling into a job,
then links each binary onto `PATH` so later steps can run it by name.

By default it installs the Docker CLI, `docker-compose`, `dockerize`, and
`goss`/`dgoss`. Each of those can be turned off, and a Docker credential helper
can be turned on.

## Usage

Invoke it inside a `run:` step with the full function path, and put `--` before
the flags:

```yaml
steps:
  - run:
      name: Install Docker tooling
      command: circleci run github.com/circleci-functions/setup-docker@<version> --
```

Install `docker-compose` only, pinned to a version:

```yaml
steps:
  - run:
      name: Install docker-compose
      command: >-
        circleci run github.com/circleci-functions/setup-docker@<version> --
        --install-docker=false --install-dockerize=false --install-goss=false
        --docker-compose-version v2.29.7
```

Install the Docker CLI and a credential helper:

```yaml
steps:
  - run:
      command: >-
        circleci run github.com/circleci-functions/setup-docker@<version> --
        --install-docker-compose=false --install-dockerize=false --install-goss=false
        --install-credential-helper --credential-helper-name pass
```

## Flags

A tool already installed at the requested version is left in place.

### What to install

| Flag | Default | Description |
|---|---|---|
| `--install-docker` | `true` | Install the Docker CLI. |
| `--install-docker-compose` | `true` | Install `docker-compose`. |
| `--install-dockerize` | `true` | Install `dockerize`. |
| `--install-goss` | `true` | Install `goss` and `dgoss` (Linux only). |
| `--install-credential-helper` | `false` | Install a Docker credential helper. |

Turn a default-on tool off with `=false`, e.g. `--install-goss=false`.

### Docker CLI

| Flag | Default | Description |
|---|---|---|
| `--version` | `latest` | Version to install, or `latest`. |
| `--install-dir` | `/usr/local/bin` | Directory to install the Docker binaries into. |

### docker-compose

| Flag | Default | Description |
|---|---|---|
| `--docker-compose-version` | `latest` | Version to install, or `latest`. |
| `--docker-compose-install-dir` | `/usr/local/bin` | Directory to install into. |

The download is verified against its published SHA-256 checksum; a mismatch
fails the step.

### dockerize

| Flag | Default | Description |
|---|---|---|
| `--dockerize-version` | `latest` | Version to install, or `latest`. |
| `--dockerize-install-dir` | `/usr/local/bin` | Directory to install into. |

### goss and dgoss

| Flag | Default | Description |
|---|---|---|
| `--goss-version` | `latest` | Version to install, or `latest`. |
| `--goss-install-dir` | `/usr/local/bin` | Directory to install `goss` and `dgoss` into. |
| `--goss-architecture` | `amd64` | Architecture to install: `amd64` or `arm64`. |

`goss` is Linux-only. When it is left on by default on a non-Linux platform it
is skipped with a warning, so the other tools still install; when it is asked
for explicitly on a non-Linux platform the step fails.

### Docker credential helper

| Flag | Default | Description |
|---|---|---|
| `--credential-helper-name` | (by platform) | `pass` or `osxkeychain`; chosen by platform when empty. |
| `--credential-helper-arch` | `amd64` | Architecture on Linux: `amd64` or `arm64`. |
| `--credential-helper-release-tag` | (latest) | Release tag to install; latest when empty. |

The helper installs into `/usr/local/bin`. Choosing `pass` also installs it and
initialises its gpg-backed store, which requires an apt-based distribution.

## Platform support

| Tool | Linux | macOS |
|---|---|---|
| Docker CLI | yes | yes |
| docker-compose | yes | yes |
| dockerize | yes | yes |
| goss / dgoss | yes | no |
| credential helper | yes (Debian/Ubuntu for `pass`) | yes |

Windows is not supported.
