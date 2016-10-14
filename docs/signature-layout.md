# Signature File Layout

A common file layout for storing and serving signatures provides a consistent way to reference image signatures. Signatures on a filesystem or a web server shall use this common layout. Signatures stored in a REST API are not required to use this common layout.

## Specification

This specification relies on [RFC3986](https://tools.ietf.org/html/rfc3986), focusing on defining a [path component](https://tools.ietf.org/html/rfc3986#section-3.3) to compose a concise URI reference to a signature.

**SCHEME[AUTHORITY]/PATH_PREFIX/IMAGE@MANIFEST_DIGEST/signature-INT**

**Definitions**

* **SCHEME**: URI scheme per [RFC3986](https://tools.ietf.org/html/rfc3986#section-3.1), e.g. **file://** or **https://**
* **AUTHORITY**: An optional authority reference per [RFC3986](https://tools.ietf.org/html/rfc3986#section-3.2), e.g. **example.com**
* **PATH_PREFIX**: An arbitrary base path to the image component
* **IMAGE**: The name of the image per [v2 API](https://docs.docker.com/registry/spec/api/#/overview). This would typically take the form of registry/repository/image but is not required to have exactly three parts. There is no requirement to include a **:PORT** component but it should be included if part of the image reference.
* **MANIFEST_DIGEST**: The value of the manifest digest, including the hash function and hash, e.g. **sha256:HASH**
* **INT**: An integer of the signature starting with 1. For multiple signatures increment by 1, e.g. **signature-1**, **signature-2**.

## Examples

1. A reference to a local file signature

        file:///var/lib/containers/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
1. A reference to a signature on a web server

        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
1. A reference to two signatures on a web server

        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-2

## Signature Indexing and Discovery

There is no signature indexing mechanism or service defined. Signatures are obtained by iterating with increasing indexes, stopping at first missing index.
