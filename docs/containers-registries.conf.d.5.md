% CONTAINERS-REGISTRIES.CONF.D 5
% Valentin Rothberg
% Mar 2020

# NAME
containers-registries.conf.d - directory for drop-in registries.conf files

# DESCRIPTION
CONTAINERS-REGISTRIES.CONF.D is a system-wide directory for drop-in
configuration files in the `containers-registries.conf(5)` format.

By default, the directory is located at `/etc/containers/registries.conf.d`.

# CONFIGURATION PRECEDENCE

Once the main configuration at `/etc/containers/registries.conf` is loaded, the
files in `/etc/containers/registries.conf.d` are loaded in alpha-numerical
order. Then the conf files in `$HOME/.config/containers/registries.conf.d` are loaded in alpha-numerical order, if they exist. If the `$HOME/.config/containers/registries.conf` is loaded, only the conf files under `$HOME/.config/containers/registries.conf.d` are loaded in alpha-numerical order.
Specified fields in a conf file will overwrite any previous setting.  Note
that only files with the `.conf` suffix are loaded, other files and
sub-directories are ignored.

For instance, setting the `unqualified-search-registries` in
`/etc/containers/registries.conf.d/myregistries.conf` will overwrite previous
settings in `/etc/containers/registries.conf`.  The `[[registry]]` tables merged
by overwriting existing items if the prefixes are identical while new ones are
added.

All drop-in configuration files must be specified in the version 2 of the
`containers-registries.conf(5)` format.

# SEE ALSO
`containers-registries.conf(5)`

# HISTORY

Mar 2020, Originally compiled by Valentin Rothberg <rothberg@redhat.com>
