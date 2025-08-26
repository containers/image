[![Go Reference](https://pkg.go.dev/badge/github.com/containers/image/v5.svg)](https://pkg.go.dev/github.com/containers/image/v5) [![Build Status](https://api.cirrus-ci.com/github/containers/image.svg)](https://cirrus-ci.com/github/containers/image)
=

> [!WARNING]
> This package was moved; please update your references to use `go.podman.io/image/v5` instead.
> New development of this project happens on https://github.com/containers/container-libs.
> For more information, check https://blog.podman.io/2025/08/upcoming-migration-of-three-containers-repositories-to-monorepo/.

`image` is a set of Go libraries aimed at working in various way with
containers' images and container image registries.

The containers/image library allows application to pull and push images from
container image registries, like the docker.io and quay.io registries. It also
implements "simple image signing".

The containers/image library also allows you to inspect a repository on a
container registry without pulling down the image. This means it fetches the
repository's manifest and it is able to show you a `docker inspect`-like json
output about a whole repository or a tag. This library, in contrast to `docker
inspect`, helps you gather useful information about a repository or a tag
without requiring you to run `docker pull`.

The containers/image library also allows you to translate from one image format
to another, for example docker container images to OCI images. It also allows
you to copy container images between various registries, possibly converting
them as necessary, and to sign and verify images.

## Command-line usage

The containers/image project is only a library with no user interface;
you can either incorporate it into your Go programs, or use the `skopeo` tool:

The [skopeo](https://github.com/containers/skopeo) tool uses the
containers/image library and takes advantage of many of its features,
e.g. `skopeo copy` exposes the `containers/image/copy.Image` functionality.

## Dependencies

This library ships as a [Go module].

## Building

If you want to see what the library can do, or an example of how it is called,
consider starting with the [skopeo](https://github.com/containers/skopeo) tool
instead.

To integrate this library into your project, include it as a [Go module],
put it into `$GOPATH` or use your preferred vendoring tool to include a copy
in your project. Ensure that the dependencies documented [in go.mod][go.mod]
are also available (using those exact versions or different versions of
your choosing).

This library also depends on some C libraries. Either install them:
```sh
Fedora$ dnf install gpgme-devel libassuan-devel
macOS$ brew install gpgme
```
or use the build tags described below to avoid the dependencies (e.g. using `go build -tags …`)

[Go module]: https://github.com/golang/go/wiki/Modules
[go.mod]: https://github.com/containers/image/blob/master/go.mod

### Supported build tags

- `containers_image_docker_daemon_stub`: Don’t import the `docker-daemon:` transport in `github.com/containers/image/transports/alltransports`, to decrease the amount of required dependencies.  Use a stub which reports that the transport is not supported instead.
- `containers_image_openpgp`: Use a Golang-only OpenPGP implementation for signature verification instead of the default cgo/gpgme-based implementation;
the primary downside is that creating new signatures with the Golang-only implementation is not supported.
- `containers_image_sequoia`: Use Sequoia-PGP for signature verification instead of the default cgo/gpgme-based or the Golang-only OpenPGP implementations, and enable the `signature/simplesequoia` subpackage. This requires a support shared library installed on the system. Install https://github.com/ueno/podman-sequoia , and potentially update build configuration to point at it (compare `SEQUOIA_SONAME_DIR` in `Makefile`).
- `containers_image_storage_stub`: Don’t import the `containers-storage:` transport in `github.com/containers/image/transports/alltransports`, to decrease the amount of required dependencies.  Use a stub which reports that the transport is not supported instead.

## [Contributing](CONTRIBUTING.md)

Information about contributing to this project.

When developing this library, please use `make` (or `make … BUILDTAGS=…`) to take advantage of the tests and validation.

## License

Apache License 2.0

SPDX-License-Identifier: Apache-2.0

## Contact

- Mailing list: [containers-dev](https://groups.google.com/forum/?hl=en#!forum/containers-dev)
- IRC: #[container-projects](irc://irc.freenode.net:6667/#container-projects) on freenode.net
