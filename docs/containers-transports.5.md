% CONTAINERS-TRANSPORTS 5 Containers Transports Man Page
% Valentin Rothberg
% April 2019

## NAME

containers-transports - description of supported transports for copying and storing container images

## DESCRIPTION

Tools which use the containers/image library, including skopeo(1), buildah(1), podman(1), all share a common syntax for referring to container images in various locations.
The general form of the syntax is _transport_`:`_details_, where details are dependent on the specified transport, which are documented below.

The semantics of the image names ultimately depend on the environment where
they are evaluated. For example: if evaluated on a remote server, image names
might refer to paths on that server; relative paths are relative to the current
directory of the image consumer.

<!-- atomic: is deprecated and not documented here. -->

### **containers-storage:**[**[**_storage-specifier_**]**]{_image-id_|_docker-reference_[**@**_image-id_]}

An image located in a local containers storage.
The format of _docker-reference_ is described in detail in the **docker** transport.

The _storage-specifier_ allows for referencing storage locations on the file system and has the format `[`[_driver_`@`]_root_[`+`_run-root_][`:`_options_]`]` where the optional _driver_ refers to the storage driver (e.g., `overlay` or `btrfs`) and where _root_ is an absolute path to the storage's root directory.
The optional _run-root_ can be used to specify the run directory of the storage where all temporary writable content is stored.
The optional _options_ are a comma-separated list of driver-specific options.
Please refer to containers-storage.conf(5) for further information on the drivers and supported options.

### **dir:**_path_

An existing local directory _path_ storing the manifest, layer tarballs and signatures as individual files.
This is a non-standardized format, primarily useful for debugging or noninvasive container inspection.

### **docker://**_docker-reference_

An image in a registry implementing the "Docker Registry HTTP API V2".
By default, uses the authorization state in `$XDG_RUNTIME_DIR/containers/auth.json`, which is set using podman-login(1).
If the authorization state is not found there, `$HOME/.docker/config.json` is checked, which is set using docker-login(1).
The containers-registries.conf(5) further allows for configuring various settings of a registry.

Note that a _docker-reference_ has the following format: _name_[`:`_tag_ | `@`_digest_].
While the docker transport does not support both a tag and a digest at the same time some formats like containers-storage do.
Digests can also be used in an image destination as long as the manifest matches the provided digest.

The docker transport supports pushing images without a tag or digest to a registry when the image name is suffixed with `@@unknown-digest@@`. The _name_`@@unknown-digest@@` reference format cannot be used with a reference that has a tag or digest.
The digest of images can be explored with skopeo-inspect(1).

If _name_ does not contain a slash, it is treated as `docker.io/library/`_name_.
Otherwise, the component before the first slash is checked if it is recognized as a _hostname_[`:`_port_] (i.e., it contains either a `.` or a `:`, or the component is exactly `localhost`).
If the first component of name is not recognized as a _hostname_[`:`_port_], _name_ is treated as `docker.io/`_name_.

### **docker-archive:**_path_[`:`{_docker-reference_|`@`_source-index_}]

An image is stored in the docker-save(1) formatted file.
_docker-reference_ must not contain a digest.
Alternatively, for reading archives, `@`_source-index_ is a zero-based index in archive manifest
(to access untagged images).
If neither _docker-reference_ nor `@`_source_index is specified when reading an archive, the archive must contain exactly one image.

The _path_ can refer to a stream, e.g. `docker-archive:/dev/stdin`.

### **docker-daemon:**_docker-reference_|_algo_`:`_digest_

An image stored in the docker daemon's internal storage.
The image must be specified as a _docker-reference_ or in an alternative _algo_`:`_digest_ format when being used as an image source.
The _algo_`:`_digest_ refers to the image ID reported by docker-inspect(1).

### **oci:**_path_[`:`_reference_]

An image in a directory structure compliant with the "Open Container Image Layout Specification" at _path_.

The _path_ value terminates at the first `:` character; any further `:` characters are not separators, but a part of _reference_.
The _reference_ is used to set, or match, the `org.opencontainers.image.ref.name` annotation in the top-level index.
If _reference_ is not specified when reading an image, the directory must contain exactly one image.

### **oci-archive:**_path_[`:`_reference_]

An image in a tar(1) archive with contents compliant with the "Open Container Image Layout Specification" at _path_.

The _path_ value terminates at the first `:` character; any further `:` characters are not separators, but a part of _reference_.
The _reference_ is used to set, or match, the `org.opencontainers.image.ref.name` annotation in the top-level index.
If _reference_ is not specified when reading an archive, the archive must contain exactly one image.

### **ostree:**_docker-reference_[`@`_/absolute/repo/path_]

An image in the local ostree(1) repository.
_/absolute/repo/path_ defaults to `/ostree/repo`.

### **sif:**_path_

An image using the Singularity image format at _path_.

Only reading images is supported, and not all scripts can be represented in the OCI format.

<!-- tarball: can only usefully be used from Go callers who call tarballReference.ConfigUpdate, and is not documented here. -->

## Examples

The following examples demonstrate how some of the containers transports can be used.
The examples use skopeo-copy(1) for copying container images.

**Copying an image from one registry to another**:
```
$ skopeo copy docker://docker.io/library/alpine:latest docker://localhost:5000/alpine:latest
```

**Copying an image from a running Docker daemon to a directory in the OCI layout**:
```
$ mkdir alpine-oci
$ skopeo copy docker-daemon:alpine:latest oci:alpine-oci
$ tree alpine-oci
test-oci/
├── blobs
│   └── sha256
│       ├── 83ef92b73cf4595aa7fe214ec6747228283d585f373d8f6bc08d66bebab531b7
│       ├── 9a6259e911dcd0a53535a25a9760ad8f2eded3528e0ad5604c4488624795cecc
│       └── ff8df268d29ccbe81cdf0a173076dcfbbea4bb2b6df1dd26766a73cb7b4ae6f7
├── index.json
└── oci-layout

2 directories, 5 files
```

**Copying an image from a registry to the local storage**:
```
$ skopeo copy docker://docker.io/library/alpine:latest containers-storage:alpine:latest
```

## SEE ALSO

docker-login(1), docker-save(1), ostree(1), podman-login(1), skopeo-copy(1), skopeo-inspect(1), tar(1), container-registries.conf(5), containers-storage.conf(5)

## AUTHORS

Miloslav Trmač <mitr@redhat.com>
Valentin Rothberg <rothberg@redhat.com>
