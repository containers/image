Name:           skopeo
Version:        0.1.2
Release:        0%{?dist}
Summary:        Inspect Docker images and repositories on registries
License:        MIT
URL:            https://github.com/runcom/skopeo
Source:         https://github.com/runcom/skopeo/archive/v%{version}.tar.gz

BuildRequires: golang >= 1.5
BuildRequires: golang-github-cpuguy83-go-md2man
BuildRequires: golang(github.com/Sirupsen/logrus) >= 0.8.4
BuildRequires: golang(github.com/codegangsta/cli) >= 1.2.0
BuildRequires: golang(golang.org/x/net/context)

%global debug_package %{nil}

%description
Get information about a Docker image or repository without pulling it

%prep
%setup -q -n %{name}-%{version}

rm -rf vendor/github.com/codegangsta
rm -rf vendor/github.com/Sirupsen
rm -rf vendor/golang.org

%build
mkdir -p ./_build/src/github.com/runcom
ln -s $(pwd) ./_build/src/github.com/runcom/skopeo
export GOPATH=$(pwd)/_build:%{gopath}
cd $(pwd)/_build/src/github.com/runcom/skopeo && make %{?_smp_mflags}

%install
mkdir -p $RPM_BUILD_ROOT/%{_mandir}/man1
make DESTDIR=%{buildroot} install

%files
%{_bindir}/skopeo
%{_mandir}/man1/skopeo.1*
%doc README.md LICENSE

%changelog
* Fri Jan 22 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.2
- v0.1.2
* Fri Jan 22 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.1
- v0.1.1
* Thu Jan 21 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.0
- v0.1.0
