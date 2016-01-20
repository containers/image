skopeo
=

_Please be aware `skopeo` is still work in progress_

`skopeo` is a command line utility which is able to _inspect_ an image from a remote Docker registry.
By _inspect_ I mean it just fetches the image's manifest and it is able to show you a `docker inspect`-like
json output. This tool, in constrast to `docker inspect`, helps you gather useful information about
an image before pulling it (and use disk space) - e.g. - which tags are available for the given image? which labels the image has?

Example:
```sh
# show image's labels
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

# show image's tags
$ skopeo fedora | jq '.RepoTags'
[
  "20",
  "21",
  "22",
  "23",
  "heisenbug",
  "latest",
  "rawhide"
]
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
- get rid of Docker (meaning make this work w/o needing Docker installed)
