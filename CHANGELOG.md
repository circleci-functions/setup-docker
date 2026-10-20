# Changelog

All notable changes to setup-docker will be documented in this file. Each release's section is added
by its release PR (see [RELEASE.md](RELEASE.md)) and becomes its GitHub release notes.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-10-19

### What's Changed
* Build and push images by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/15
* Merge per-platform builds into one multi-platform tag by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/16
* Lint Dockerfiles with hadolint by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/17
* Build Windows container images with the classic builder by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/18


**Full Changelog**: https://github.com/circleci-functions/setup-docker/compare/v0.2.0...v0.3.0

## [0.2.0] - 2026-10-16

### What's Changed
* Log in to container registries by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/12
* Run the login and remote Docker smoke checks as acceptance tests by @stiyyagura0901 in https://github.com/circleci-functions/setup-docker/pull/14


**Full Changelog**: https://github.com/circleci-functions/setup-docker/compare/v0.1.0...v0.2.0

## [0.1.0]

First release: installs the Docker CLI, buildx and compose, and optionally dockerize and goss.
