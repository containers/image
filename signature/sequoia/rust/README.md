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
$ make RELEASE=1
```

## Installing

Just copy the shared library in the library search path:

```console
$ sudo cp -a rust/target/release/libimage_sequoia.so* /usr/lib64
```

## Testing

To test, do:
```console
$ LD_LIBRARY_PATH=rust/target/release \

[sequoia-pgp]: https://sequoia-pgp.org/
