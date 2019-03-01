% containers-certs.d(5)

# NAME
containers-certs.d - Directory for storing custom container-registry certficates

# DESCRIPTION
A custom certificate for a container registry can be configured by creating a directory under `/etc/containers/certs.d`.
The name of the directory must correspond to the `host:port` of the registry (e.g., `my-registry.com:5000`).

## Directory Structure
A certs directory can contain one or more CA roots (i.e., `.crt` files) and one or more `<client>.{cert,key}` cert pairs.
If more than one root or pair is specified, they will be used in alpha-numerical order.

```
/etc/containers/certs.d/    <- Certificate directory
└── my-registry.com:5000    <- Hostname:port
   ├── client.cert          <- Client certificate
   ├── client.key           <- Client key
   └── ca.crt               <- Certificate authority that signed the registry certificate
```

### Creating Client Certificates
A client certificate pair can be generated via `openssl`:

```
$ openssl genrsa -out client.key 4096
$ openssl req -new -x509 -text -key client.key -out client.cert
```

# SEE ALSO
openssl(1)

# HISTORY
Feb 2019, Originally compiled by Valentin Rothberg <rothberg@redhat.com>
