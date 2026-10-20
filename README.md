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

## Logging in

Use `setup-docker/login` to authenticate with a container registry after installing Docker.

**Docker Hub** (default env names):

```yaml
      - setup-docker/login
```

**GitHub Container Registry** with custom env names:

```yaml
      - setup-docker/login:
          with:
            registry: ghcr.io
            username-env: GHCR_USER
            password-env: GHCR_TOKEN
```

**Amazon ECR** after a step that sets up AWS credentials (no username or password needed):

```yaml
      - setup-docker/login:
          with:
            registry: 123456789012.dkr.ecr.us-east-1.amazonaws.com
```

ECR login uses `docker-credential-ecr-login`, which picks up AWS credentials from the
environment automatically. No `DOCKER_LOGIN` or `DOCKER_PASSWORD` are read.

### Arguments for `setup-docker/login`

| Argument | Default | Description |
|------|---------|-------------|
| `registry` | `docker.io` | Registry to log in to. |
| `username-env` | `DOCKER_LOGIN` | Name of the environment variable holding the username. |
| `password-env` | `DOCKER_PASSWORD` | Name of the environment variable holding the password or token. |
| `retries` | `3` | Times to retry a failed login, with increasing waits. |
| `credentials-store` | | Store credentials with a credential helper instead of in the Docker config file. |
| `credentials-helper` | | Credential helper to use (`pass` or `osxkeychain`). Empty picks `pass` on Linux and `osxkeychain` on macOS. |
| `credentials-helper-version` | | Credential helper version. Empty installs the pinned version. |

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

## Building and pushing

Use `setup-docker/build` after `setup-docker` (or `setup-docker/login` for authenticated
registries) to build and push images with buildx.

**Docker Hub after login:**

```yaml
      - setup-docker/login
      - setup-docker/build:
          with:
            image: myorg/app
            tags: latest
            push: true
```

**GHCR multi-arch:**

```yaml
      - setup-docker/build:
          with:
            image: ghcr.io/myorg/app
            tags: latest
            platforms: linux/amd64,linux/arm64
            push: true
```

**Caching between jobs:** turn on Docker Layer Caching (DLC). `build` uses a builder
named `setup-docker`, which DLC keeps between jobs, so BuildKit starts with the previous
job's layers:

```yaml
      - setup_remote_docker:
          docker_layer_caching: true
      - setup-docker/build:
          with:
            image: myorg/app
            tags: latest
```

On a `machine` executor, set `docker_layer_caching: true` under `machine`. To keep the
cache in a registry instead, use `cache-from` and `cache-to`:

```yaml
      - setup-docker/build:
          with:
            image: myorg/app
            tags: latest
            cache-from: type=registry,ref=myorg/app:cache
            cache-to: type=registry,ref=myorg/app:cache,mode=max
            push: true
```

**Check, SBOM, push-by-digest, and build-args:**

```yaml
      - setup-docker/build:
          with:
            image: myorg/app
            tags: latest
            check: true
            sbom: true
            sbom-file: /tmp/sbom.json
            push-by-digest: true
            digest-file: /tmp/digest.txt
            build-args: |
              APP_VERSION=1.2.3
              BUILD_DATE=2025-01-01
```

### Arguments for `setup-docker/build`

| Argument | Default | Description |
|---|---|---|
| `image` | (required) | Image to build, e.g. `myorg/app` or `ghcr.io/org/app`. |
| `registry` | `docker.io` | Registry prefixed to image when image names no registry. |
| `tags` | | Tags, comma or newline separated. Empty uses `$CIRCLE_SHA1`. |
| `dockerfile` | | Path to the Dockerfile. Empty uses the `Dockerfile` in `context`. |
| `context` | `.` | Build context directory. |
| `target` | | Build stage to stop at. |
| `platforms` | | Target platforms, comma separated, e.g. `linux/amd64,linux/arm64`. |
| `push` | | Push the image after building. |
| `load` | `auto` | Load into the Docker engine. `auto` loads single-platform non-push builds. |
| `build-args` | | Build arguments, one `KEY=VALUE` per line. |
| `labels` | | Image labels, one `KEY=VALUE` per line. |
| `secret-envs` | | Build secrets from env vars, comma or newline separated. |
| `pull` | | Always pull newer base images. |
| `no-cache` | | Disable Docker's build cache. |
| `cache-from` | | Extra cache sources, whitespace separated. |
| `cache-to` | | Extra cache destinations, whitespace separated. |
| `provenance` | | Attach a provenance attestation. |
| `sbom` | | Attach an SBOM attestation. Needs `push` or `sbom-file`. |
| `sbom-file` | | Also write the SBOM (SPDX JSON) to this path. |
| `check` | | Run Dockerfile build checks first. Warnings are printed; errors fail the build. |
| `check-warnings-as-errors` | | With `check`, fail on warnings too. |
| `push-by-digest` | | Push without tags; write the digest to `digest-file`. |
| `digest-file` | | Write the image digest to this path. |
| `buildkitd-config` | | Path to a `buildkitd.toml`, e.g. for a registry mirror or plain HTTP. |
| `extra-args` | | Extra `docker buildx build` arguments. |
| `retries` | `3` | Times to retry a build that fails while pushing. |

### Platform note

On Windows executors, `build` builds Windows container images with Docker's classic
builder. The following arguments are not available on Windows: `platforms`, `cache-from`,
`cache-to`, `sbom`, `sbom-file`, `provenance`, `secret-envs`, `check`, `push-by-digest`,
`buildkitd-config`, and `load=false`.

For Linux images, use a docker executor with `setup_remote_docker`, a machine executor,
or point `DOCKER_HOST` at a Linux daemon. macOS jobs also need `DOCKER_HOST`.

For multi-platform builds, `build` registers QEMU binfmt handlers for foreign platforms
and removes them after the build. Handlers that were already registered before the build
are left untouched.

## Merging per-platform builds

`setup-docker/merge` combines per-platform images that were each pushed by digest into
one multi-arch manifest index. This lets each architecture build natively in its own
job, then a final job publishes a single tag that works everywhere.

```yaml
version: 2.1

functions:
  setup-docker: github.com/circleci-functions/setup-docker@v0.1.0

jobs:
  build-amd64:
    machine:
      image: ubuntu-2404:current
    steps:
      - checkout
      - setup-docker/build:
          with:
            image: myorg/app
            platforms: linux/amd64
            push-by-digest: true
            digest-file: digests/amd64
      - persist_to_workspace:
          root: .
          paths: [digests]

  build-arm64:
    machine:
      image: ubuntu-2404:current
    resource_class: arm.medium
    steps:
      - checkout
      - setup-docker/build:
          with:
            image: myorg/app
            platforms: linux/arm64
            push-by-digest: true
            digest-file: digests/arm64
      - persist_to_workspace:
          root: .
          paths: [digests]

  merge:
    docker:
      - image: cimg/base:stable
    steps:
      - attach_workspace:
          at: .
      - setup-docker/login
      - setup-docker/merge:
          with:
            image: myorg/app
            tags: latest,1.2.3

workflows:
  main:
    jobs:
      - build-amd64
      - build-arm64
      - merge:
          requires: [build-amd64, build-arm64]
```

The merge job needs no Docker daemon (`setup_remote_docker` is not required).

### Arguments for `setup-docker/merge`

| Argument | Default | Description |
|------|---------|-------------|
| `image` | | Image to publish, e.g. `myorg/app` or `ghcr.io/org/app`. Required. |
| `registry` | `docker.io` | Registry prefixed to image when image names no registry. |
| `tags` | | Tags, comma or newline separated. Empty uses `$CIRCLE_SHA1`. |
| `digests-dir` | `digests` | Directory of digest files written by build with `push-by-digest`. |
| `digest-file` | | Write the digest of the merged image to this path. |
| `retries` | `3` | Times to retry a failed merge. |

## Linting Dockerfiles

Use `setup-docker/lint` to lint Dockerfiles with [hadolint](https://github.com/hadolint/hadolint).
Results can be collected for the Test Summary tab via `store_test_results`.

```yaml
      - setup-docker/lint:
          with:
            dockerfiles: Dockerfile
            failure-threshold: warning
            ignore-rules: DL3008,SC2086
            trusted-registries: docker.io
            results-file: test-results/hadolint.xml
      - store_test_results:
          path: test-results
```

### Arguments for `setup-docker/lint`

| Argument | Default | Description |
|------|---------|-------------|
| `dockerfiles` | `Dockerfile` | Dockerfiles to lint, newline or colon separated (semicolon on Windows). |
| `failure-threshold` | `info` | Fail on findings at or above this level: `error`, `warning`, `info`, `style`, `ignore`, `none`. As in hadolint, `style`, `ignore` and `none` fail on any finding. |
| `ignore-rules` | | Rules to ignore, comma separated, e.g. `DL3008,SC2086`. |
| `trusted-registries` | | Registries base images may come from, comma separated. Others are reported. |
| `results-file` | | Also write the findings as JUnit XML to this path, for `store_test_results`. |
| `hadolint-version` | | hadolint version, or `latest`. Empty installs the pinned version. |

## Platform support

Linux and macOS on amd64 and arm64. Windows amd64 is supported for the Docker CLI,
buildx, and compose. Docker publishes no Windows arm64 CLI, so only buildx and compose
are available on windows/arm64. On Alpine, the Alpine build of dockerize is used
automatically. dockerize and goss have no Windows builds and return an error if requested
on Windows.

On macOS there is no Docker daemon; the CLI and plugins install and work, but running
containers requires a remote daemon.
