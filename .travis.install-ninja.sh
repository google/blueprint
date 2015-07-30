#!/bin/bash

# Version of ninja to build -- can be any git revision
VERSION="v1.6.0"

set -ev

SCRIPT_HASH=$(sha1sum ${BASH_SOURCE[0]} | awk '{print $1}')

cd ~
if [[ -d ninjabin && "$SCRIPT_HASH" == "$(cat ninjabin/script_hash)" ]]; then
  exit 0
fi

git clone https://github.com/martine/ninja
cd ninja
./configure.py --bootstrap

mkdir -p ../ninjabin
rm -f ../ninjabin/ninja
echo -n $SCRIPT_HASH >../ninjabin/script_hash
mv ninja ../ninjabin/
