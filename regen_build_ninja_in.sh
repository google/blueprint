#!/bin/bash

SRC=$(cd $(dirname $0) && pwd)
OUT=/tmp/blueprint.$$

rm -rf ${OUT}
mkdir ${OUT}

(
  cd ${OUT}
  ${SRC}/bootstrap.bash
  ./blueprint.bash
  ${SRC}/bootstrap.bash -r
)

rm -rf ${OUT}
mkdir ${OUT}

(
  cd ${OUT}
  cp -r ${SRC}/tests/test_tree test_tree
  ln -s ${SRC} test_tree/blueprint
  mkdir out
  cd out

  export SRCDIR=../test_tree
  ${SRCDIR}/blueprint/bootstrap.bash
  ./blueprint.bash
  cp .minibootstrap/build.ninja.in ${SRC}/tests/test_tree/build.ninja.in
)

rm -rf ${OUT}
