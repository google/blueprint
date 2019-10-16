#simplebp

Simplebp is a minimalistic custom bootstrapping build system on top of
Blueprint. It has support for compiling C and C++ code, linking
executables and shared libraries and running scripts. The main purpose
of simplebp is to demonstrate how Blueprint works.

Simplebp was originally written by Tim Kilbourn; see
https://github.com/tkilbourn/simplebp.

Build and run:

- `git submodule update --init`
  The example includes Blueprint source as a Git submodule, because
  otherwise it would need to reference sourse files outside of the top
  source directory or use a symlink, both of which does not work well
  with Blueprint.

- `mkdir out && cd out`
  Create the output directory and cd into it.

- `../bootstrap.bash`
  Run the bootstrapping script that will set some varaibles required
  by Blueprint and start the Blueprint bootstrapping process, which
  will build `minibp` (the minimal Blueprint binary that will further
  build the real build system). It will also create `blueprint.bash`
  script in the current directory, which is the driver for the build
  system.

- `./blueprint.bash`
  Run the build. This will build source code for both the build system
  (in ../build directory) and the hello-world program (../hello).

- `./hello/hello`
  Run the resulting hello-world program. Not that library paths are
  currently hard-coded in the executable's RPATH, so can only be run
  from the top build directory (or with LD_LIBRARY_PATH).

- Modify different parts of the source code in ../build and ../hello,
  re-run `./blueprint.bash` and watch the buld system in action.

Have fun! ;]
