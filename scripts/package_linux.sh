#!/usr/bin/env bash
set -euo pipefail

# Packaging script for Linux: creates tar.gz, .deb and .rpm (if fpm available)
# Usage: VERSION=1.2.3 ./scripts/package_linux.sh

VERSION=${VERSION:-0.0.0}
BINARY_NAME="pancakeos"
DIST_DIR="dist"
PKG_DIR="package"

mkdir -p "$DIST_DIR"
rm -rf "$PKG_DIR"

echo "Building linux/amd64 binary..."
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "$DIST_DIR/$BINARY_NAME" .

# 1) tar.gz
TAR_NAME="${BINARY_NAME}_${VERSION}_linux_amd64.tar.gz"
rm -f "$DIST_DIR/$TAR_NAME"
(
  cd "$DIST_DIR"
  tar -czf "$TAR_NAME" "$BINARY_NAME"
)
echo "Created $DIST_DIR/$TAR_NAME"

# 1.5) AppImage
# Use absolute path for APPDIR to avoid issues when switching cwd
APPDIR="$(pwd)/$PKG_DIR/AppDir"
echo "Preparing AppDir for AppImage..."
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin"
mkdir -p "$APPDIR/usr/share/icons/hicolor/256x256/apps"
cp "$DIST_DIR/$BINARY_NAME" "$APPDIR/usr/bin/$BINARY_NAME"
chmod 0755 "$APPDIR/usr/bin/$BINARY_NAME"

# AppRun
cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/bash
DIR="$(dirname "$(readlink -f "$0")")"
exec "$DIR/usr/bin/pancakeos" "$@"
EOF
chmod 0755 "$APPDIR/AppRun"

# Desktop file
cat > "$APPDIR/pancakeos.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=PancakeOS
Exec=pancakeos
Icon=pancakeos
Categories=Utility;
EOF

# Minimal 1x1 PNG icon (placeholder)
echo "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR4nGNgYAAAAAMAASsJTYQAAAAASUVORK5CYII=" | base64 -d > "$APPDIR/pancakeos.png"
cp "$APPDIR/pancakeos.png" "$APPDIR/usr/share/icons/hicolor/256x256/apps/pancakeos.png"

# Build AppImage using appimagetool if available, otherwise download it
APPIMAGETOOL="$(command -v appimagetool || true)"
APPIMAGE_NAME="${BINARY_NAME}-${VERSION}-x86_64.AppImage"
if [ -x "$APPIMAGETOOL" ]; then
  echo "appimagetool found: $APPIMAGETOOL"
  "$APPIMAGETOOL" "$APPDIR" "$DIST_DIR/$APPIMAGE_NAME"
  echo "Created $DIST_DIR/$APPIMAGE_NAME"
else
  echo "appimagetool not found — downloading appimagetool to /tmp"
  TOOL_TMP="/tmp/appimagetool-x86_64.AppImage"
  if [ ! -f "$TOOL_TMP" ]; then
    curl -L -o "$TOOL_TMP" "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage"
    chmod +x "$TOOL_TMP"
  fi
  # run from /tmp to ensure executable FS
  # AppImage may require FUSE; extract and run the inner appimagetool binary as fallback
  CWD=$(pwd)
  cd /tmp
  OUT_TMP="/tmp/$APPIMAGE_NAME"
  if "$TOOL_TMP" --appimage-extract >/dev/null 2>&1; then
    if [ -x ./squashfs-root/usr/bin/appimagetool ]; then
      ARCH=x86_64 ./squashfs-root/usr/bin/appimagetool "$APPDIR" "$OUT_TMP"
      mv "$OUT_TMP" "$DIST_DIR/" || true
      echo "Created $DIST_DIR/$APPIMAGE_NAME"
      rm -rf ./squashfs-root
      cd "$CWD"
    else
      echo "Inner appimagetool not found after extraction; attempting to run AppImage directly"
      cd "$CWD"
      ARCH=x86_64 "$TOOL_TMP" "$APPDIR" "$OUT_TMP"
      mv "$OUT_TMP" "$DIST_DIR/" || true
      echo "Created $DIST_DIR/$APPIMAGE_NAME"
    fi
  else
    echo "Extraction not supported; attempting to run AppImage directly"
    cd "$CWD"
    ARCH=x86_64 "$TOOL_TMP" "$APPDIR" "$OUT_TMP"
    mv "$OUT_TMP" "$DIST_DIR/" || true
    echo "Created $DIST_DIR/$APPIMAGE_NAME"
  fi
fi

# 2) DEB
DEB_OUT="${BINARY_NAME}_${VERSION}_amd64.deb"
PKG_DEB_ROOT="$PKG_DIR/debroot"
DEB_INSTALL_DIR="$PKG_DEB_ROOT/opt/${BINARY_NAME}"

mkdir -p "$DEB_INSTALL_DIR/bin"
cp "$DIST_DIR/$BINARY_NAME" "$DEB_INSTALL_DIR/bin/"
chmod 0755 "$DEB_INSTALL_DIR/bin/$BINARY_NAME"

# generate control file from template if present
CTRL_TEMPLATE="packaging/debian/control.tpl"
CONTROL_DEST="$PKG_DEB_ROOT/DEBIAN"
# create DEBIAN control directory with safe permissions (dpkg-deb requires 0755..0775)
mkdir -p "$CONTROL_DEST"
chmod 0755 "$PKG_DEB_ROOT"
chmod 0755 "$CONTROL_DEST"
if [ -f "$CTRL_TEMPLATE" ]; then
  sed "s/{{VERSION}}/$VERSION/g" "$CTRL_TEMPLATE" > "$CONTROL_DEST/control"
else
  cat > "$CONTROL_DEST/control" <<EOF
Package: pancakeos
Version: $VERSION
Section: utils
Priority: optional
Architecture: amd64
Maintainer: PancakeOS <noreply@example.com>
Description: PancakeOS launcher
 A small launcher for PancakeOS.
EOF
fi
chmod 0644 "$CONTROL_DEST/control"

if command -v dpkg-deb >/dev/null 2>&1; then
  # Some filesystems (e.g. /mnt/c) do not support Unix permissions properly.
  # Build the .deb in a temporary native FS directory to ensure dpkg-deb sees correct perms.
  TMPROOT=$(mktemp -d)
  trap 'rm -rf "$TMPROOT"' EXIT
  cp -a "$PKG_DEB_ROOT/." "$TMPROOT/"
  chmod -R u=rwX,go=rX "$TMPROOT"
  # Ensure DEBIAN dir perms
  if [ -d "$TMPROOT/DEBIAN" ]; then
    chmod 0755 "$TMPROOT/DEBIAN"
    [ -f "$TMPROOT/DEBIAN/control" ] && chmod 0644 "$TMPROOT/DEBIAN/control"
  fi
  dpkg-deb --build "$TMPROOT" "$DIST_DIR/$DEB_OUT"
  echo "Created $DIST_DIR/$DEB_OUT"
  rm -rf "$TMPROOT"
else
  echo "dpkg-deb not found; .deb build skipped. To build locally install dpkg-deb or run on a Debian/Ubuntu host."
fi

# 3) RPM
RPM_OUT="${BINARY_NAME}-${VERSION}-1.x86_64.rpm"
# Prefer fpm if available
if command -v fpm >/dev/null 2>&1; then
  echo "fpm found, building RPM using fpm..."
  # fpm will pick up the files under PKG_DEB_ROOT/opt/pancakeos
  (cd "$PKG_DEB_ROOT" && fpm -s dir -t rpm -n "$BINARY_NAME" -v "$VERSION" --architecture x86_64 --prefix / -C . opt)
  # fpm outputs to cwd; move RPM to dist if present
  mv *.rpm "$DIST_DIR/" 2>/dev/null || true
  echo "RPM(s) moved to $DIST_DIR"
else
  echo "fpm not found; attempting rpm with rpmbuild (may require rpmbuild and Linux environment)."
  RPMBUILD_DIR="$(pwd)/rpmbuild"
  mkdir -p "$RPMBUILD_DIR"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
  SPEC_TEMPLATE="packaging/rpm/pancakeos.spec.tpl"
  SPEC_FILE="$RPMBUILD_DIR/SPECS/pancakeos.spec"
  if [ -f "$SPEC_TEMPLATE" ]; then
    sed "s/{{VERSION}}/$VERSION/g" "$SPEC_TEMPLATE" > "$SPEC_FILE"
  else
    cat > "$SPEC_FILE" <<EOF
Name: pancakeos
Version: $VERSION
Release: 1
Summary: PancakeOS launcher
License: Proprietary
Group: Applications/System
BuildArch: x86_64

%description
PancakeOS launcher

%prep

%build

%install
mkdir -p %{buildroot}/opt/pancakeos/bin
cp "$DIST_DIR/$BINARY_NAME" %{buildroot}/opt/pancakeos/bin/

%files
/opt/pancakeos/bin/$BINARY_NAME

%changelog
* $(date -R) PancakeOS Packager
- Initial
EOF
  fi
  echo "You can try: rpmbuild --define '_topdir $RPMBUILD_DIR' -bb $SPEC_FILE"
  echo "RPM build skipped in this script when fpm is not available."
fi

echo "Packaging complete. Dist directory: $DIST_DIR"

exit 0
