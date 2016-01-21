Name:           skopeo
Version:        0.1.0
Release:        1%{?dist}
Summary:        Inspect Docker images and repositories on registries
License:        MIT
URL:            https://github.com/runcom/skopeo
Source:         https://github.com/runcom/skopeo/archive/v%{version}.tar.gz

BuildRequires: golang
BuildRequires: golang-github-cpuguy83-go-md2man

%global debug_package %{nil}

%description
Get information about a Docker image or repository without pulling it

%prep
%setup -q -n %{name}-%{version}

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
* Thu Jan 21 2016 Antonio Murdaca <runcom@redhat.com> - 0.1.0
- v0.1.0
