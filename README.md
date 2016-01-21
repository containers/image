skopeo
=

_Please be aware `skopeo` is still work in progress_

`skopeo` is a command line utility which is able to _inspect_ a repository on a Docker registry.
By _inspect_ I mean it fetches the repository's manifest and it is able to show you a `docker inspect`-like
json output about a whole repository or a tag. This tool, in constrast to `docker inspect`, helps you gather useful information about
a repository or a tag before pulling it (using disk space) - e.g. - which tags are available for the given repository? which labels the image has?

Example:
```sh
# show repository's labels of rhel7:latest
$ skopeo registry.access.redhat.com/rhel7 | jq '.Config.Labels'
{
  "Architecture": "x86_64",
  "Authoritative_Registry": "registry.access.redhat.com",
  "BZComponent": "rhel-server-docker",
  "Build_Host": "rcm-img04.build.eng.bos.redhat.com",
  "Name": "rhel7/rhel",
  "Release": "38",
  "Vendor": "Red Hat, Inc.",
  "Version": "7.2"
}

# show repository's tags
$ skopeo docker.io/fedora | jq '.RepoTags'
[
  "20",
  "21",
  "22",
  "23",
  "heisenbug",
  "latest",
  "rawhide"
]

# show image's digest
$ skopeo docker.io/fedora:rawhide | jq '.Digest'
"sha256:905b4846938c8aef94f52f3e41a11398ae5b40f5855fb0e40ed9c157e721d7f8"
```
Building
-
```sh
$ git clone https://github.com/runcom/skopeo
$ make
```
Installing
-
```sh
$ sudo make install
```
License
-
MIT
TODO
-
- show repo tags via flag or when reference isn't tagged or digested
- add tests (integration with deployed registries in container - Docker-like)
- get rid of Docker (meaning make this work w/o needing Docker installed)

NOT TODO
-
- provide a _format_ flag - just use the awesome [jq](https://stedolan.github.io/jq/)
