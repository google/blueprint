if [ ! "${BLUEPRINT_BOOTSTRAP_VERSION}" -eq "1" ]; then
  echo "Please run bootstrap.bash again (out of date)" >&2
  exit 1
fi

export GOROOT

source "${BLUEPRINTDIR}/microfactory/microfactory.bash"

BUILDDIR="${BUILDDIR}/.minibootstrap" build_go minibp github.com/google/blueprint/bootstrap/minibp

# Build the bootstrap build.ninja
"${NINJA}" -w dupbuild=err -f "${BUILDDIR}/.minibootstrap/build.ninja"

# Build the primary builder and the main build.ninja
"${NINJA}" -w dupbuild=err -f "${BUILDDIR}/.bootstrap/build.ninja"

# SKIP_NINJA can be used by wrappers that wish to run ninja themselves.
if [ -z "$SKIP_NINJA" ]; then
    "${NINJA}" -w dupbuild=err -f "${BUILDDIR}/build.ninja" "$@"
else
    exit 0
fi
