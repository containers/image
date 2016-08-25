# OCI Image Signature Specification

**Version 0.1**

## Introduction

This document specifies an out-of-band cryptographic verification model for
container images. Detached signatures are associated with images by manifest digest hash value.

## Signature Format

### Fields

Field Name | Type | Description
---|:---:|---
<a name="critical"></a>critical | [ [CriticalObject](#criticalObject) ] | **Required.**
<a name="optional"></a>optional | [ [OptionalObject](#optionalObject) ] | **Optional.**

#### <a name="criticalObject"></a>Critical Object

Field Name | Type | Description
---|:---:|---
<a name="identity"></a>identity | [ [IdentityObject](#identityObject) ] | **Required.**
<a name="image"></a>image | [ [ImageObject](#imageObject) ] | **Required.**
<a name="type"></a>type | `string` | **Required.** Valid values: "atomic container signature"

#### <a name="identityObject"></a>Identity Object

Field Name | Type | Description
---|:---:|---
<a name="docker-reference"></a>docker-reference | `string` | **Required.** Image name

#### <a name="imageObject"></a>Image Object

Field Name | Type | Description
---|:---:|---
<a name="docker-manifest-digest"></a>docker-manifest-digest | `digest` | **Required.** Manifest digest in the form of `<algorithm>:<hashValue>`

#### <a name="optionalObject"></a>Optional Object

Field Name | Type | Description
---|:---:|---
<a name="creator"></a>creator | `string` | **Optional.** Creator ID
<a name="timestamp"></a>timestamp | `int64` | **Optional.** Timestamp epoch

### Example

See [example signature file](signature.json).

### Encryption and Decryption

The signature data is written to a file that is encrypted with a private key. The file may be decrypted using the corresponding public key.

**Example GPG encrypt command**

TODO

**Example GPG decrypt command**

```
$ gpg --decrypt busybox.sig | python -m json.tool
```

## Static File Layout

When encrypted signature files are stored as static files the following directory layout shall be used:

        [/<pathPrefix>]/<registryUri>/<repository>/<image@sha256:hash>/<pubkeyID>

## Validation

1. Decrypt signature file.
1. Compare image manifest digest with signature digest

## Policy

Policy provides a way to describe trust by mapping registries, repositories and images to a list of required public keys. It also describes default fallback behavior when conditions are not met.

Policy answers the following questions:

* What should be done when an image does not have specific policy?
* What list of public keys do you trust for a given registry, repository or image?

See [policy specification file](policy.go).

**Example Policy File**

https://github.com/projectatomic/skopeo/blob/master/integration/fixtures/policy.json

## Signature Server Discovery

To enable a reasonable user experience, a mechanism is needed to map a given registry with a signature server and associated public keys. Without such a mechanism each user would have to look up this information out-of-band. While this can lead to insecure workflows, Discovery allows tooling to prompt users to make a trust evaluation: Do you want to trust this public key for this registry?

### Discovery Container Format

A container image may be used to provide registry metadata. The image uses LABELs with signature server metadata. Four LABELs are specified.

#### Discovery Container Naming

Discovery container images shall have the following characteristics:

* The image shall be named **sigstore**.
* The primary image shall be tagged **:latest**
* When more than one image is served arbitrary tags may be used, such as **:auxilliary** or **:backup**.
* The scope of the signature metadata shall apply to the image being served. In other words, an image served at *registry.example.com/acme/sigstore:latest* provides information about images in the "acme" repository namespace. An image served at *registry.example.com/sigstore:latest* provides information about all images in the registry.example.com registry.

**Example Dockerfile**

Consider the following Dockerfile built as registry.example.com/acme/sigstore:latest and pushed to the registry.

```
FROM scratch

LABEL sigstore-url="https://sigstore.example.com:8443" \
      pubkey-id="ef442d51: Example, Inc. <security@example.com>" \
      pubkey-fingerprint="657F 347A D004 4ADE 55BA 8A5F 199E 2F91 FD43 1D51" \
      pubkey-download-url="https://www.example.com/security/ef431d51.txt"
```
