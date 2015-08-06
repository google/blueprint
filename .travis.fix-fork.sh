#!/bin/bash

if echo $TRAVIS_BUILD_DIR | grep -vq "github.com/google/blueprint$" ; then
  cd ../..
  mkdir -p google
  mv $TRAVIS_BUILD_DIR google/blueprint
  cd google/blueprint
  export TRAVIS_BUILD_DIR=$PWD
fi
