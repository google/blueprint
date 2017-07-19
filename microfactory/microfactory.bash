# Copyright 2017 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Set of utility functions to build and run go code with microfactory
#
# Inputs:
#  ${GOROOT}
#  ${BUILDDIR}
#  ${BLUEPRINTDIR}
#  ${SRCDIR}

# Bootstrap microfactory from source if necessary and use it to build the
# requested binary.
#
# Arguments:
#  $1: name of the requested binary
#  $2: package name
#  ${EXTRA_ARGS}: extra arguments to pass to microfactory (-pkg-path, etc)
function build_go
{
    # Increment when microfactory changes enough that it cannot rebuild itself.
    # For example, if we use a new command line argument that doesn't work on older versions.
    local mf_version=2

    local mf_src="${BLUEPRINTDIR}/microfactory"
    local mf_bin="${BUILDDIR}/microfactory_$(uname)"
    local mf_version_file="${BUILDDIR}/.microfactory_$(uname)_version"
    local built_bin="${BUILDDIR}/$1"
    local from_src=1

    if [ -f "${mf_bin}" ] && [ -f "${mf_version_file}" ]; then
        if [ "${mf_version}" -eq "$(cat "${mf_version_file}")" ]; then
            from_src=0
        fi
    fi

    local mf_cmd
    if [ $from_src -eq 1 ]; then
        mf_cmd="${GOROOT}/bin/go run ${mf_src}/microfactory.go"
    else
        mf_cmd="${mf_bin}"
    fi

    rm -f "${BUILDDIR}/.$1.trace"
    # GOROOT must be absolute because `go run` changes the local directory
    GOROOT=$(cd $GOROOT; pwd) ${mf_cmd} -s "${mf_src}" -b "${mf_bin}" \
            -pkg-path "github.com/google/blueprint=${BLUEPRINTDIR}" \
            -trimpath "${SRCDIR}" \
            ${EXTRA_ARGS} \
            -o "${built_bin}" $2

    if [ $from_src -eq 1 ]; then
        echo "${mf_version}" >"${mf_version_file}"
    fi
}
