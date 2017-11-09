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
#   BLUEPRINTDIR
#   BUILDDIR
#   NINJA_BUILDDIR
#   GOROOT
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

# BLUEPRINTDIR should be set to the path to the blueprint source. It generally
# should start with SRCDIR.
[ -z "$BLUEPRINTDIR" ] && BLUEPRINTDIR="${SRCDIR}"

# BUILDDIR should be set to the path to store build results. By default, this
# is the current directory, but it may be set to an absolute or relative path.
[ -z "$BUILDDIR" ] && BUILDDIR=.

# NINJA_BUILDDIR should be set to the path to store the .ninja_log/.ninja_deps
# files. By default this is the same as $BUILDDIR.
[ -z "$NINJA_BUILDDIR" ] && NINJA_BUILDDIR="${BUILDDIR}"

# TOPNAME should be set to the name of the top-level Blueprints file
[ -z "$TOPNAME" ] && TOPNAME="Blueprints"

# These variables should be set by auto-detecting or knowing a priori the host
# Go toolchain properties.
[ -z "$GOROOT" ] && GOROOT=`go env GOROOT`

usage() {
    echo "Usage of ${BOOTSTRAP}:"
    echo "  -h: print a help message and exit"
    echo "  -b <builddir>: set the build directory"
    echo "  -t: run tests"
}

# Parse the command line flags.
while getopts ":b:ht" opt; do
    case $opt in
        b) BUILDDIR="$OPTARG";;
        t) RUN_TESTS=true;;
        h)
            usage
            exit 1
            ;;
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

# If RUN_TESTS is set, behave like -t was passed in as an option.
[ ! -z "$RUN_TESTS" ] && EXTRA_ARGS="${EXTRA_ARGS} -t"

# Allow the caller to pass in a list of module files
if [ -z "${BLUEPRINT_LIST_FILE}" ]; then
  BLUEPRINT_LIST_FILE="${BUILDDIR}/.bootstrap/bplist"
fi
EXTRA_ARGS="${EXTRA_ARGS} -l ${BLUEPRINT_LIST_FILE}"

mkdir -p $BUILDDIR/.minibootstrap

echo "bootstrapBuildDir = $BUILDDIR" > $BUILDDIR/.minibootstrap/build.ninja
echo "topFile = $SRCDIR/$TOPNAME" >> $BUILDDIR/.minibootstrap/build.ninja
echo "extraArgs = $EXTRA_ARGS" >> $BUILDDIR/.minibootstrap/build.ninja
echo "builddir = $NINJA_BUILDDIR" >> $BUILDDIR/.minibootstrap/build.ninja
echo "include $BLUEPRINTDIR/bootstrap/build.ninja" >> $BUILDDIR/.minibootstrap/build.ninja

echo "BLUEPRINT_BOOTSTRAP_VERSION=2" > $BUILDDIR/.blueprint.bootstrap
echo "SRCDIR=\"${SRCDIR}\"" >> $BUILDDIR/.blueprint.bootstrap
echo "BLUEPRINTDIR=\"${BLUEPRINTDIR}\"" >> $BUILDDIR/.blueprint.bootstrap
echo "NINJA_BUILDDIR=\"${NINJA_BUILDDIR}\"" >> $BUILDDIR/.blueprint.bootstrap
echo "GOROOT=\"${GOROOT}\"" >> $BUILDDIR/.blueprint.bootstrap
echo "TOPNAME=\"${TOPNAME}\"" >> $BUILDDIR/.blueprint.bootstrap

touch "${BUILDDIR}/.out-dir"

if [ ! -z "$WRAPPER" ]; then
    cp $WRAPPER $BUILDDIR/
fi
