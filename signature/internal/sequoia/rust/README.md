# image-sequoia

This directory contains the source code of a C shared library
(`libimage_sequoia.so`) that enables to use [sequoia-pgp] as a signing
backend.

For building, you need rustc (version 1.79 or later), cargo, and
openssl-devel. For testing, you also need the `sq` command (version
1.3.0 or later).

## Building

To build the shared library and bindings, do:

```console
$ PREFIX=/usr LIBDIR="\${prefix}/lib64" cargo build --release
```

## Installing

Just copy the shared library in the library search path:

```console
$ sudo cp -a rust/target/release/libimage_sequoia.so* /usr/lib64
```

## Testing

To test, in the top-level directory of containers image, do:
```console
$ LD_LIBRARY_PATH=$PWD/signature/internal/sequoia/rust/target/release \
  make BUILDTAGS=containers_image_sequoia
```
[sequoia-pgp]: https://sequoia-pgp.org/
