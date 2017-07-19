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
#  ${TOP}: The top of the android source tree
#  ${OUT_DIR}: The output directory location (defaults to ${TOP}/out)
#  ${OUT_DIR_COMMON_BASE}: Change the default out directory to
#    ${OUT_DIR_COMMON_BASE}/$(basename ${TOP})

# Ensure GOROOT is set to the in-tree version.
case $(uname) in
    Linux)
        export GOROOT="${TOP}/prebuilts/go/linux-x86/"
        ;;
    Darwin)
        export GOROOT="${TOP}/prebuilts/go/darwin-x86/"
        ;;
    *) echo "unknown OS:" $(uname) >&2 && exit 1;;
esac

# Find the output directory
function getoutdir
{
    local out_dir="${OUT_DIR-}"
    if [ -z "${out_dir}" ]; then
        if [ "${OUT_DIR_COMMON_BASE-}" ]; then
            out_dir="${OUT_DIR_COMMON_BASE}/$(basename ${TOP})"
        else
            out_dir="out"
        fi
    fi
    if [[ "${out_dir}" != /* ]]; then
        out_dir="${TOP}/${out_dir}"
    fi
    echo "${out_dir}"
}

# Bootstrap microfactory from source if necessary and use it to build the
# requested binary.
#
# Arguments:
#  $1: name of the requested binary
#  $2: package name
function build_go
{
    # Increment when microfactory changes enough that it cannot rebuild itself.
    # For example, if we use a new command line argument that doesn't work on older versions.
    local mf_version=2

    local mf_src="${TOP}/build/soong/cmd/microfactory"

    local out_dir=$(getoutdir)
    local mf_bin="${out_dir}/microfactory_$(uname)"
    local mf_version_file="${out_dir}/.microfactory_$(uname)_version"
    local built_bin="${out_dir}/$1"
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

    rm -f "${out_dir}/.$1.trace"
    ${mf_cmd} -s "${mf_src}" -b "${mf_bin}" \
            -pkg-path "android/soong=${TOP}/build/soong" -trimpath "${TOP}/build/soong" \
            -o "${built_bin}" $2

    if [ $from_src -eq 1 ]; then
        echo "${mf_version}" >"${mf_version_file}"
    fi
}
