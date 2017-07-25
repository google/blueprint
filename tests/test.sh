#!/bin/bash -ex

# Go to srcdir
cd $(dirname ${BASH_SOURCE[0]})/..

rm -rf out.test
mkdir out.test
cd out.test
../tests/bootstrap.bash
./blueprint.bash

if [[ -d .bootstrap/blueprint/test ]]; then
  echo "Tests should not be enabled here" >&2
  exit 1
fi

sleep 2
../tests/bootstrap.bash -t
./blueprint.bash

if [[ ! -d .bootstrap/blueprint/test ]]; then
  echo "Tests should be enabled here" >&2
  exit 1
fi

sleep 2
../tests/bootstrap.bash
./blueprint.bash

if [[ -d .bootstrap/blueprint/test ]]; then
  echo "Tests should not be enabled here (2)" >&2
  exit 1
fi
