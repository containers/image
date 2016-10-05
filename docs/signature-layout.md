# Signature File Layout

A common file layout for storing and serving signatures provides a consistent way to reference image signatures. Signatures on a filesystem or a web server shall use this common layout. Signatures stored in a REST API are not required to use this common layout.

## Specification

**SCHEME://[URI]/PATH_PREFIX/REGISTRY/REPOSITORY/IMAGE@MANIFEST_DIGEST/signature-INT**

* **SCHEME**: The transport scheme, e.g. **file://** or **https://**
* **URI**: For remote transport schemes, the signature server URI, e.g. **example.com**
* **PATH_PREFIX**: The path to the base of the registry directory
* **REGISTRY**: The registry URI and optional port, e.g. **registry.example.com:5000**
* **REPOSITORY**: The repository namespace. May occur multiple times for registries that support multiple repository namespaces.
* **IMAGE**: The name of the image
* **MANIFEST_DIGEST**: The value of the manifest digest, including the hash function and hash, e.g. **sha256:HASH**
* **INT**: An integer of the signature starting with 1. For multiple signatures increment by 1, e.g. **signature-1**, **signature-2**.

## Examples

1. A reference to a local file signature

        file:///var/lib/atomic/sigstore/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
1. A reference to a signature on a web server

        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
1. A reference to two signatures on a web server

        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-1
        https://sigs.example.com/signatures/registry.example.com:5000/acme/myimage@sha256:b1c302ecc8e21804a288491cedfed9bd3db972ac8367ccab7340b33ecd1cb8eb/signature-2
