#!/bin/bash

export BOOTSTRAP="${BASH_SOURCE[0]}"
export SRCDIR=".."
export BOOTSTRAP_MANIFEST="src.build.ninja.in"

../bootstrap.bash "$@"
