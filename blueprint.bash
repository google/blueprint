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
#   NINJA
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

# NINJA should be set to the path of the ninja executable. By default, this
# is just "ninja", and will be looked up in $PATH.
[ -z "$NINJA" ] && NINJA=ninja


if [ ! -f "${BUILDDIR}/.blueprint.bootstrap" ]; then
    echo "Please run bootstrap.bash (.blueprint.bootstrap missing)" >&2
    exit 1
fi

# .blueprint.bootstrap provides saved values from the bootstrap.bash script:
#
#   BLUEPRINT_BOOTSTRAP_VERSION
#   BLUEPRINTDIR
#   SRCDIR
#   GOROOT
#
source "${BUILDDIR}/.blueprint.bootstrap"

if [ -z "$BLUEPRINTDIR" ]; then
    echo "Please run bootstrap.bash (.blueprint.bootstrap outdated)" >&2
    exit 1
fi

source "${BLUEPRINTDIR}/blueprint_impl.bash"
