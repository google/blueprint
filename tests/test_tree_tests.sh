#!/bin/bash -ex

function mtime() {
    stat -c %Y $1
}

# Go to top of blueprint tree
cd $(dirname ${BASH_SOURCE[0]})/..
TOP=${PWD}

export TEMPDIR=$(mktemp -d -t blueprint.test.XXX)

function cleanup() {
    cd "${TOP}"
    echo "cleaning up ${TEMPDIR}"
    rm -rf "${TEMPDIR}"
}
trap cleanup EXIT

export OUTDIR="${TEMPDIR}/out"
mkdir "${OUTDIR}"

export SRCDIR="${TEMPDIR}/src"
cp -r tests/test_tree "${SRCDIR}"
cp -r "${TOP}" "${SRCDIR}/blueprint"

cd "${OUTDIR}"
export BLUEPRINTDIR=${SRCDIR}/blueprint
#setup
${SRCDIR}/blueprint/bootstrap.bash $@

#confirm no build.ninja file is rebuilt when no change happens
./blueprint.bash

OLDTIME_BOOTSTRAP=$(mtime .bootstrap/build.ninja)
OLDTIME=$(mtime build.ninja)

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for null build" >&2
    exit 1
fi

if [ ${OLDTIME_BOOTSTRAP} != $(mtime .bootstrap/build.ninja) ]; then
    echo "unnecessary .bootstrap/build.ninja regeneration for null build" >&2
    exit 1
fi

#confirm no build.ninja file is rebuilt when a new directory is created
mkdir ${SRCDIR}/newglob

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for new empty directory" >&2
    exit 1
fi
if [ ${OLDTIME_BOOTSTRAP} != $(mtime .bootstrap/build.ninja) ]; then
    echo "unnecessary .bootstrap/build.ninja regeneration for new empty directory" >&2
    exit 1
fi

#confirm that build.ninja is rebuilt when a new Blueprints file is added
touch ${SRCDIR}/newglob/Blueprints

sleep 2
./blueprint.bash

if [ ${OLDTIME} = $(mtime build.ninja) ]; then
    echo "Failed to rebuild build.ninja for glob addition" >&2
    exit 1
fi
if [ ${OLDTIME_BOOTSTRAP} = $(mtime .bootstrap/build.ninja) ]; then
    echo "Failed to rebuild .bootstrap/build.ninja for glob addition" >&2
    exit 1
fi

#confirm that build.ninja is rebuilt when a glob match is removed
OLDTIME=$(mtime build.ninja)
OLDTIME_BOOTSTRAP=$(mtime .bootstrap/build.ninja)
rm ${SRCDIR}/newglob/Blueprints

sleep 2
./blueprint.bash

if [ ${OLDTIME} = $(mtime build.ninja) ]; then
    echo "Failed to rebuild build.ninja for glob removal" >&2
    exit 1
fi
if [ ${OLDTIME_BOOTSTRAP} = $(mtime .bootstrap/build.ninja) ]; then
    echo "Failed to rebuild .bootstrap/build.ninja for glob removal" >&2
    exit 1
fi

#confirm that build.ninja is not rebuilt when a glob match is removed
OLDTIME=$(mtime build.ninja)
OLDTIME_BOOTSTRAP=$(mtime .bootstrap/build.ninja)
rmdir ${SRCDIR}/newglob

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for removal of empty directory" >&2
    exit 1
fi

if [ ${OLDTIME_BOOTSTRAP} != $(mtime .bootstrap/build.ninja) ]; then
    echo "unnecessary .bootstrap/build.ninja regeneration for removal of empty directory" >&2
    exit 1
fi

echo tests passed
