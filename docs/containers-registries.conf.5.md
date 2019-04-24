% CONTAINERS-REGISTRIES.CONF(5) System-wide registry configuration file
% Brent Baude
% Aug 2017

# NAME
containers-registries.conf - Syntax of System Registry Configuration File

# DESCRIPTION
The CONTAINERS-REGISTRIES configuration file is a system-wide configuration
file for container image registries. The file format is TOML.

By default, the configuration file is located at `/etc/containers/registries.conf`.

# FORMATS

## VERSION 2
VERSION 2 is the latest format of the `registries.conf` and is currently in
beta. This means in general VERSION 1 should be used in production environments
for now.

Every registry can have its own mirrors configured.  The mirrors will be tested
in order for the availability of the remote manifest.  This happens currently
only during an image pull.  If the manifest is not reachable due to connectivity
issues or the unavailability of the remote manifest, then the next mirror will
be tested instead.  If no mirror is configured or contains the manifest to be
pulled, then the initially provided reference will be used as fallback.  It is
possible to set the `insecure` option per mirror, too.

Furthermore it is possible to specify a `prefix` for a registry.  The `prefix`
is used to find the relevant target registry from where the image has to be
pulled.  During the test for the availability of the image, the prefixed
location will be rewritten to the correct remote location.  This applies to
mirrors as well as the fallback `location`.  If no prefix is specified, it
defaults to the specified `location`.  For example, if
`prefix = "example.com/foo"`, `location = "example.com"` and the image will be
pulled from `example.com/foo/image`, then the resulting pull will be effectively
point to `example.com/image`.

By default container runtimes use TLS when retrieving images from a registry.
If the registry is not setup with TLS, then the container runtime will fail to
pull images from the registry. If you set `insecure = true` for a registry or a
mirror you overwrite the `insecure` flag for that specific entry.  This means
that the container runtime will attempt use unencrypted HTTP to pull the image.
It also allows you to pull from a registry with self-signed certificates.

If you set the `unqualified-search = true` for the registry, then it is possible
to omit the registry hostname when pulling images.  This feature does not work
together with a specified `prefix`.

If `blocked = true` then it is not allowed to pull images from that registry.

### EXAMPLE

```
[[registry]]
location = "example.com"
insecure = false
prefix = "example.com/foo"
unqualified-search = false
blocked = false
mirror = [
    { location = "example-mirror-0.local", insecure = false },
    { location = "example-mirror-1.local", insecure = true }
]
```

## VERSION 1
VERSION 1 can be used as alternative to the VERSION 2, but it is not capable in
using registry mirrors or a prefix.

The TOML_format is used to build a simple list for registries under three
categories: `registries.search`, `registries.insecure`, and `registries.block`.
You can list multiple registries using a comma separated list.

Search registries are used when the caller of a container runtime does not fully specify the
container image that they want to execute.  These registries are prepended onto the front
of the specified container image until the named image is found at a registry.

Note insecure registries can be used for any registry, not just the registries listed
under search.

The fields `registries.insecure` and `registries.block` work as like as the
`insecure` and `blocked` from VERSION 2.

### EXAMPLE
The following example configuration defines two searchable registries, one
insecure registry, and two blocked registries.

```
[registries.search]
registries = ['registry1.com', 'registry2.com']

[registries.insecure]
registries = ['registry3.com']

[registries.block]
registries = ['registry.untrusted.com', 'registry.unsafe.com']
```

# HISTORY
Mar 2019, Added additional configuration format by Sascha Grunert <sgrunert@suse.com>

Aug 2018, Renamed to containers-registries.conf(5) by Valentin Rothberg <vrothberg@suse.com>

Jun 2018, Updated by Tom Sweeney <tsweeney@redhat.com>

Aug 2017, Originally compiled by Brent Baude <bbaude@redhat.com>
