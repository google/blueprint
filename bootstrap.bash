#!/bin/bash

# This script serves two purposes.  First, it can bootstrap the standalone
# Blueprint to generate the minibp binary.  To do this simply run the script
# with no arguments from the desired build directory.
#
# It can also be invoked from another script to bootstrap a custom Blueprint-
# based build system.  To do this, the invoking script must first set some or
# all of the following environment variables, which are documented below where
# their default values are set:
#
#   BOOTSTRAP
#   WRAPPER
#   SRCDIR
#   BUILDDIR
#   BOOTSTRAP_MANIFEST
#   GOROOT
#   GOOS
#   GOARCH
#   GOCHAR
#
# The invoking script should then run this script, passing along all of its
# command line arguments.

set -e

EXTRA_ARGS=""

# BOOTSTRAP should be set to the path of the bootstrap script.  It can be
# either an absolute path or one relative to the build directory (which of
# these is used should probably match what's used for SRCDIR).
if [ -z "$BOOTSTRAP" ]; then
    BOOTSTRAP="${BASH_SOURCE[0]}"

    # WRAPPER should only be set if you want a ninja wrapper script to be
    # installed into the builddir. It is set to blueprint's blueprint.bash
    # only if BOOTSTRAP and WRAPPER are unset.
    [ -z "$WRAPPER" ] && WRAPPER="`dirname "${BOOTSTRAP}"`/blueprint.bash"
fi

# SRCDIR should be set to the path of the root source directory.  It can be
# either an absolute path or a path relative to the build directory.  Whether
# its an absolute or relative path determines whether the build directory can
# be moved relative to or along with the source directory without re-running
# the bootstrap script.
[ -z "$SRCDIR" ] && SRCDIR=`dirname "${BOOTSTRAP}"`

# BUILDDIR should be set to the path to store build results. By default, this
# is the current directory, but it may be set to an absolute or relative path.
[ -z "$BUILDDIR" ] && BUILDDIR=.

# TOPNAME should be set to the name of the top-level Blueprints file
[ -z "$TOPNAME" ] && TOPNAME="Blueprints"

# BOOTSTRAP_MANIFEST is the path to the bootstrap Ninja file that is part of
# the source tree.  It is used to bootstrap a build output directory from when
# the script is run manually by a user.
[ -z "$BOOTSTRAP_MANIFEST" ] && BOOTSTRAP_MANIFEST="${SRCDIR}/build.ninja.in"

# These variables should be set by auto-detecting or knowing a priori the host
# Go toolchain properties.
[ -z "$GOROOT" ] && GOROOT=`go env GOROOT`
[ -z "$GOOS" ]   && GOOS=`go env GOHOSTOS`
[ -z "$GOARCH" ] && GOARCH=`go env GOHOSTARCH`
[ -z "$GOCHAR" ] && GOCHAR=`go env GOCHAR`

# If RUN_TESTS is set, behave like -t was passed in as an option.
[ ! -z "$RUN_TESTS" ] && EXTRA_ARGS="$EXTRA_ARGS -t"

GOTOOLDIR="$GOROOT/pkg/tool/${GOOS}_$GOARCH"
GOCOMPILE="$GOTOOLDIR/${GOCHAR}g"
GOLINK="$GOTOOLDIR/${GOCHAR}l"

if [ ! -f $GOCOMPILE ]; then
  GOCOMPILE="$GOTOOLDIR/compile"
fi
if [ ! -f $GOLINK ]; then
  GOLINK="$GOTOOLDIR/link"
fi
if [[ ! -f $GOCOMPILE || ! -f $GOLINK ]]; then
  echo "Cannot find go tools under $GOROOT"
  exit 1
fi

usage() {
    echo "Usage of ${BOOTSTRAP}:"
    echo "  -h: print a help message and exit"
    echo "  -r: regenerate ${BOOTSTRAP_MANIFEST}"
    echo "  -t: include tests when regenerating manifest"
}

# Parse the command line flags.
IN="$BOOTSTRAP_MANIFEST"
REGEN_BOOTSTRAP_MANIFEST=false
while getopts ":b:hi:rt" opt; do
    case $opt in
        b) BUILDDIR="$OPTARG";;
        h)
            usage
            exit 1
            ;;
        i) IN="$OPTARG";;
        r) REGEN_BOOTSTRAP_MANIFEST=true;;
        t) EXTRA_ARGS="$EXTRA_ARGS -t";;
        \?)
            echo "Invalid option: -$OPTARG" >&2
            usage
            exit 1
            ;;
        :)
            echo "Option -$OPTARG requires an argument." >&2
            exit 1
            ;;
    esac
done

if [ $REGEN_BOOTSTRAP_MANIFEST = true ]; then
    # This assumes that the script is being run from a build output directory
    # that has been built in the past.
    if [ -x $BUILDDIR/.bootstrap/bin/minibp ]; then
        echo "Regenerating $BOOTSTRAP_MANIFEST"
        $BUILDDIR/.bootstrap/bin/minibp $EXTRA_ARGS -o $BOOTSTRAP_MANIFEST $SRCDIR/$TOPNAME
    else
        echo "Executable minibp not found at $BUILDDIR/.bootstrap/bin/minibp" >&2
        exit 1
    fi
fi

mkdir -p $BUILDDIR

sed -e "s|@@SrcDir@@|$SRCDIR|g"                        \
    -e "s|@@BuildDir@@|$BUILDDIR|g"                    \
    -e "s|@@GoRoot@@|$GOROOT|g"                        \
    -e "s|@@GoCompile@@|$GOCOMPILE|g"                  \
    -e "s|@@GoLink@@|$GOLINK|g"                        \
    -e "s|@@Bootstrap@@|$BOOTSTRAP|g"                  \
    -e "s|@@BootstrapManifest@@|$BOOTSTRAP_MANIFEST|g" \
    $IN > $BUILDDIR/build.ninja

echo "BOOTSTRAP=\"${BOOTSTRAP}\"" > $BUILDDIR/.blueprint.bootstrap
echo "BOOTSTRAP_MANIFEST=\"${BOOTSTRAP_MANIFEST}\"" >> $BUILDDIR/.blueprint.bootstrap

if [ ! -z "$WRAPPER" ]; then
    cp $WRAPPER $BUILDDIR/
fi
