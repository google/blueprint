Blueprint Build System
======================

Blueprint is being archived on 2021 May 3.

On 2021 May 3, we will be archiving the Blueprint project. This means it will
not be possible to file new issues or open new pull requests for this GitHub
project. As the project is being archived, patches -- including security
patches -- will not be applied after May 3. The source tree will remain
available, but changes to Blueprint in AOSP will not be merged here and
Blueprint's source tree in AOSP will eventually stop being usable outside of
Android.

Whereas there are no meta-build systems one can use as a drop-in replacement for
Blueprint, there are a number of build systems that can be used:

* [Bazel](https://bazel.build), Google's multi-language build tool to build and
  test software of any size, quickly and reliably
* [Soong](https://source.android.com/setup/build), for building the Android
  operating system itself
* [CMake](https://cmake.org), an open-source, cross-platform family of tools
  designed to build, test and package software
* [Buck](https://buck.build), a fast build system that encourages the creation
  of small, reusable modules over a variety of platforms and languages
* The venerable [GNU Make](https://www.gnu.org/software/make/)
