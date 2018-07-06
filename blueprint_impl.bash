if [ ! "${BLUEPRINT_BOOTSTRAP_VERSION}" -eq "2" ]; then
  echo "Please run bootstrap.bash again (out of date)" >&2
  exit 1
fi


# Allow the caller to pass in a list of module files
if [ -z "$BLUEPRINT_LIST_FILE" ]; then
  # If the caller does not pass a list of module files, then do a search now
  OUR_LIST_FILE="${BUILDDIR}/.bootstrap/bplist"
  TEMP_LIST_FILE="${OUR_FILES_LIST}.tmp"
  mkdir -p "$(dirname ${OUR_LIST_FILE})"
  (cd "$SRCDIR";
    find . -mindepth 1 -type d \( -name ".*" -o -execdir test -e {}/.out-dir \; \) -prune \
      -o -name $TOPNAME -print | sort) >"${TEMP_LIST_FILE}"
  if cmp -s "${OUR_LIST_FILE}" "${TEMP_LIST_FILE}"; then
    rm "${TEMP_LIST_FILE}"
  else
    mv "${TEMP_LIST_FILE}" "${OUR_LIST_FILE}"
  fi
  BLUEPRINT_LIST_FILE="${OUR_LIST_FILE}"
fi

export GOROOT
export BLUEPRINT_LIST_FILE

source "${BLUEPRINTDIR}/microfactory/microfactory.bash"

BUILDDIR="${BUILDDIR}/.minibootstrap" build_go minibp github.com/google/blueprint/bootstrap/minibp

BUILDDIR="${BUILDDIR}/.minibootstrap" build_go bpglob github.com/google/blueprint/bootstrap/bpglob

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
