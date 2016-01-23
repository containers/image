%global commit          7c8a3fdbe04f9d4f60ad951ff2bc0c99000e4110
%global shortcommit     %(c=%{commit}; echo ${c:0:7})

Name:           skopeo
Version:        0.1.1
Release:        1%{?dist}
Summary:        Inspect Docker images and repositories on registries
License:        MIT
URL:            https://github.com/runcom/skopeo
Source:         https://github.com/runcom/skopeo/archive/v%{version}.tar.gz

BuildRequires: golang >= 1.5
BuildRequires: golang-github-cpuguy83-go-md2man
BuildRequires: golang(github.com/Sirupsen/logrus) >= 0.8.4
BuildRequires: golang(github.com/codegangsta/cli) >= 1.2.0
BuildRequires: golang(golang.org/x/net)

%global debug_package %{nil}

%description
Get information about a Docker image or repository without pulling it

%prep
%setup -q -n %{name}-%{version}

rm -rf vendor/github.com/!(docker|opencontainers|vbatts|gorilla)
rm -rf vendor/golang.org/

%build
mkdir -p src/github.com/runcom
ln -s ../../../ src/github.com/runcom/skopeo
export GOPATH=$(pwd):%{gopath}
make %{?_smp_mflags}

%install
mkdir -p $RPM_BUILD_ROOT/%{_mandir}/man1
make DESTDIR=%{buildroot} install

%files
%{_bindir}/skopeo
%{_mandir}/man1/skopeo.1*
%doc README.md LICENSE

%changelog
* Fri Jan 22 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.1
- v0.1.1
* Thu Jan 21 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.0
- v0.1.0
