% CONTAINERS-REGISTRIES.CONF.D(5)
% Valentin Rothberg
% Mar 2020

# NAME
containers-registries.conf.d - directory for drop-in registries.conf files

# DESCRIPTION
CONTAINERS-REGISTRIES.CONF.D is a systemd-wide directory for drop-in
configuration files in the `containers-registries.conf(5)` format.

By default, the directory is located at `/etc/containers/registries.conf.d`.

# CONFIGURATION PRECEDENCE

Once the main configuration at `/etc/containers/registries.conf` is loaded, the
files in `/etc/containers/registries.conf.d` are loaded in alpha-numerical order.
Specified fields in a config will overwrite any previous setting.

For instance, setting the `unqualified-search-registries` in
`/etc/containers/registries.conf.d/myregistries.conf` will overwrite previous
settings in `/etc/containers/registries.conf`.

Note that all files must be specified in the same version of the
`containers-registries.conf(5)` format.  The entire `[[registry]]` table will
always be overridden if set.

# SEE ALSO
`containers-registries.conf(5)`

# HISTORY

Mar 2020, Originally compiled by Valentin Rothberg <rothberg@redhat.com>
