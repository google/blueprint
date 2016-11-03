#!/bin/bash -ex

# Go to srcdir
cd $(dirname ${BASH_SOURCE[0]})/..

rm -rf out.test
mkdir out.test
cd out.test
cp ../build.ninja.in src.build.ninja.in
../tests/bootstrap.bash

./blueprint.bash

if [[ -d .bootstrap/blueprint/test ]]; then
  echo "Tests should not be enabled here" >&2
  exit 1
fi

sleep 2
sed -i 's/extra =/extra = -t/' src.build.ninja.in
./blueprint.bash

if [[ ! -d .bootstrap/blueprint/test ]]; then
  echo "Tests should be enabled here" >&2
  exit 1
fi

if cmp -s src.build.ninja.in .minibootstrap/build.ninja.in; then
  echo "src.build.ninja.in and .minibootstrap/build.ninja.in should be different" >&2
  exit 1
fi

sleep 2
cp ../build.ninja.in src.build.ninja.in
./blueprint.bash

if [[ -d .bootstrap/blueprint/test ]]; then
  echo "Tests should not be enabled here (2)" >&2
  exit 1
fi

if ! cmp -s src.build.ninja.in .minibootstrap/build.ninja.in; then
  echo "src.build.ninja.in and .minibootstrap/build.ninja.in should be the same" >&2
  exit 1
fi
