# setup-docker

Installs the Docker CLI, the buildx and compose plugins, and optionally dockerize and
goss, for the rest of the job.

```yaml
version: 2.1

functions:
  setup-docker: github.com/circleci-functions/setup-docker@v0.1.0

jobs:
  test:
    docker:
      - image: cimg/base:current
    steps:
      - checkout
      - setup_remote_docker
      - setup-docker:
          with:
            compose-version: v2.29.7
      - run: docker compose up --abort-on-container-exit

workflows:
  main:
    jobs:
      - test
```

To skip Docker and buildx (already on the image) and add dockerize and goss:

```yaml
      - setup-docker:
          with:
            install-docker: false
            install-buildx: false
            install-dockerize: true
            install-goss: true
```

## Choosing a version

Each `*-version` flag takes an exact version string or `latest`. Left empty, a tool
already on `PATH` is kept as-is; if nothing is installed, the version pinned in this
release is used. An exact match already on `PATH` is kept; a different version is
installed alongside and takes precedence — the image's copy is never removed.

Each release pins one version of each tool. Editors with CircleCI config support show the pinned version in each argument's description.

## How tools reach later steps

Binaries are stored under `~/.cache/setup-docker/<tool>/<version>` and linked into a
directory already on `PATH` (or `install-dir`), so later steps find them by name in
any shell. buildx and compose are written to the Docker CLI plugin directory
(`$DOCKER_CONFIG/cli-plugins`, defaulting to `~/.docker/cli-plugins`), which the Docker
CLI searches automatically; compose is also linked onto `PATH` as `docker-compose`.
Nothing is written to `$BASH_ENV`.

## Verification

Release assets are checked against the checksums each project publishes.
`download.docker.com` publishes no checksums for the Docker CLI, so only the pinned
default Docker version is verified (its checksums ship with this function); any other
Docker CLI version is downloaded over HTTPS with a warning.

## Remote Docker

With `setup_remote_docker`, this function sets up the Docker client only. The engine
version is controlled by `setup_remote_docker`, not by `docker-version`.

## Arguments

Passed under `with:`.

| Argument | Default | Description |
|------|---------|-------------|
| `install-docker` | `true` | Install the Docker CLI. |
| `install-buildx` | `true` | Install the buildx plugin. |
| `install-compose` | `true` | Install the compose plugin, also on `PATH` as `docker-compose`. |
| `install-dockerize` | `false` | Install dockerize. |
| `install-goss` | `false` | Install goss and dgoss. |
| `docker-version` | | Docker CLI version, or `latest`. Empty keeps the docker on `PATH`, else installs the pinned version. |
| `buildx-version` | | buildx version, or `latest`. Empty keeps an installed buildx, else installs the pinned version. |
| `compose-version` | | compose version, or `latest`. Empty keeps an installed compose, else installs the pinned version. |
| `dockerize-version` | | dockerize version, or `latest`. Empty keeps dockerize on `PATH`, else installs the pinned version. |
| `goss-version` | | goss version, or `latest`. Empty keeps goss on `PATH`, else installs the pinned version. |
| `install-dir` | | Link binaries into this directory instead of one already on `PATH`. |

## Platform support

Linux and macOS, on amd64 and arm64. On Alpine, the Alpine build of dockerize is used
automatically. Windows is not supported.

On macOS there is no Docker daemon; the CLI and plugins install and work, but running
containers requires a remote daemon.
