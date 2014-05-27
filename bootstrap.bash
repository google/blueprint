#!/bin/bash

# SRCDIR should be set to the path of the root source directory.  It can be
# either an absolute path or a path relative to the build directory.  Whether
# its an absolute or relative path determines whether the build directory can be
# moved relative to or along with the source directory without re-running the
# bootstrap script.
SRCDIR=`dirname "${BASH_SOURCE[0]}"`

# BOOTSTRAP should be set to the path of this script.  It can be either an
# absolute path or one relative to the build directory (which of these is used
# should probably match what's used for SRCDIR).
BOOTSTRAP="${BASH_SOURCE[0]}"

# These variables should be set by auto-detecting or knowing a priori the Go
# toolchain properties.
GOROOT=`go env GOROOT`
GOOS=`go env GOHOSTOS`
GOARCH=`go env GOHOSTARCH`
GOCHAR=`go env GOCHAR`

case "$#" in
    1) IN="$1";BOOTSTRAP_MANIFEST="$1";;
	2) IN="$1";BOOTSTRAP_MANIFEST="$2";;
    *) IN="${SRCDIR}/build.ninja.in";BOOTSTRAP_MANIFEST="$IN";;
esac

sed -e "s|@@SrcDir@@|$SRCDIR|g"                        \
    -e "s|@@GoRoot@@|$GOROOT|g"                        \
    -e "s|@@GoOS@@|$GOOS|g"                            \
    -e "s|@@GoArch@@|$GOARCH|g"                        \
    -e "s|@@GoChar@@|$GOCHAR|g"                        \
    -e "s|@@Bootstrap@@|$BOOTSTRAP|g"                  \
    -e "s|@@BootstrapManifest@@|$BOOTSTRAP_MANIFEST|g" \
    $IN > build.ninja