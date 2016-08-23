Blueprint Build System
======================
[![Build Status](https://travis-ci.org/google/blueprint.svg?branch=master)](https://travis-ci.org/google/blueprint) 

Blueprint is a meta-build system that reads in Blueprints files that describe
modules that need to be built, and produces a
[Ninja](https://ninja-build.org/) manifest describing the commands that
need to be run and their dependencies.  Where most build systems use built-in
rules or a domain-specific language to describe the logic for converting module
descriptions to build rules, Blueprint delegates this to per-project build
logic written in Go.  For large, heterogenous projects this allows the inherent
complexity of the build logic to be maintained in a high-level language, while
still allowing simple changes to individual modules by modifying easy to
understand Blueprints files.
