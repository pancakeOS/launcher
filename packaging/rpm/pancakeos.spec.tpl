Name: pancakeos
Version: {{VERSION}}
Release: 1
Summary: PancakeOS launcher
License: MIT
Group: Applications/System
BuildArch: x86_64

%description
PancakeOS launcher

%prep

%build

%install
mkdir -p %{buildroot}/opt/pancakeos/bin
cp %{_sourcedir}/pancakeos %{buildroot}/opt/pancakeos/bin/

%files
/opt/pancakeos/bin/pancakeos

%changelog
* TODO PancakeOS Packager - initial
