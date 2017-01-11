#!/bin/bash -ex

function mtime() {
    stat -c %Y $1
}

# Go to top of blueprint tree
TOP=$(dirname ${BASH_SOURCE[0]})/..
cd ${TOP}

rm -rf out.test
mkdir out.test

rm -rf src.test
mkdir src.test
cp -r tests/test_tree src.test/test_tree
ln -s ../.. src.test/test_tree/blueprint

cd out.test
export SRCDIR=../src.test/test_tree
${SRCDIR}/blueprint/bootstrap.bash
./blueprint.bash

if ! cmp -s ${SRCDIR}/build.ninja.in .minibootstrap/build.ninja.in; then
    echo "tests/test_tree/build.ninja.in and .minibootstrap/build.ninja.in should be the same" >&2
    exit 1
fi

OLDTIME=$(mtime build.ninja)

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for null build" >&2
    exit 1
fi

mkdir ${SRCDIR}/newglob

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for glob addition" >&2
    exit 1
fi

touch ${SRCDIR}/newglob/Blueprints

sleep 2
./blueprint.bash

if [ ${OLDTIME} = $(mtime build.ninja) ]; then
    echo "Failed to rebuild for glob addition" >&2
    exit 1
fi

OLDTIME=$(mtime build.ninja)
rm ${SRCDIR}/newglob/Blueprints

sleep 2
./blueprint.bash

if [ ${OLDTIME} = $(mtime build.ninja) ]; then
    echo "Failed to rebuild for glob removal" >&2
    exit 1
fi

OLDTIME=$(mtime build.ninja)
rmdir ${SRCDIR}/newglob

sleep 2
./blueprint.bash

if [ ${OLDTIME} != $(mtime build.ninja) ]; then
    echo "unnecessary build.ninja regeneration for glob removal" >&2
    exit 1
fi
