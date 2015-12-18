#!/bin/bash

# This script is intented to wrap the execution of ninja so that we
# can do some checks before each ninja run.
#
# It can either be run with a standalone Blueprint checkout to generate
# the minibp binary, or can be used by another script as part of a custom
# Blueprint-based build system. When used by another script, the following
# environment variables can be set to configure this script, which are
# documented below:
#
#   BUILDDIR
#   SKIP_NINJA
#
# When run in a standalone Blueprint checkout, bootstrap.bash will install
# this script into the $BUILDDIR, where it may be executed.
#
# For embedding into a custom build system, the current directory when this
# script executes should be the same directory that $BOOTSTRAP should be
# called from.

set -e

# BUILDDIR should be set to the path to store build results. By default,
# this is the directory containing this script, but can be set explicitly
# if the custom build system only wants to install their own wrapper.
[ -z "$BUILDDIR" ] && BUILDDIR=`dirname "${BASH_SOURCE[0]}"`

# .blueprint.bootstrap provides saved values from the bootstrap.bash script:
#
#   BOOTSTRAP
#   BOOTSTRAP_MANIFEST
#
# If it doesn't exist, we probably just need to re-run bootstrap.bash, which
# ninja will do when switching stages. So just skip to ninja.
if [ -f "${BUILDDIR}/.blueprint.bootstrap" ]; then
    source "${BUILDDIR}/.blueprint.bootstrap"

    # Pick the newer of .bootstrap/bootstrap.ninja.in or $BOOTSTRAP_MANIFEST,
    # and copy it to .bootstrap/build.ninja.in
    GEN_BOOTSTRAP_MANIFEST="${BUILDDIR}/.bootstrap/bootstrap.ninja.in"
    if [ -f "${GEN_BOOTSTRAP_MANIFEST}" ]; then
        if [ "${GEN_BOOTSTRAP_MANIFEST}" -nt "${BOOTSTRAP_MANIFEST}" ]; then
            BOOTSTRAP_MANIFEST="${GEN_BOOTSTRAP_MANIFEST}"
        fi
    fi

    # Copy the selected manifest to $BUILDDIR/.bootstrap/build.ninja.in
    mkdir -p "${BUILDDIR}/.bootstrap"
    cp "${BOOTSTRAP_MANIFEST}" "${BUILDDIR}/.bootstrap/build.ninja.in"

    # Bootstrap it to $BUILDDIR/build.ninja
    "${BOOTSTRAP}" -i "${BUILDDIR}/.bootstrap/build.ninja.in"
fi

# SKIP_NINJA can be used by wrappers that wish to run ninja themselves.
if [ -z "$SKIP_NINJA" ]; then
    ninja -C "${BUILDDIR}" "$@"
else
    exit 0
fi
