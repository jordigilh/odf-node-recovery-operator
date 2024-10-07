#!/usr/bin/env bash
set -x
set -o errexit
set -o nounset
set -o pipefail

if ! which go >/dev/null; then
  echo "Error: go not found in path"
  exit 1
fi

if ! which jq >/dev/null; then
  echo "Error: jq not found in path"
  exit 1
fi

cd $(dirname "${BASH_SOURCE[0]}")/..

if [ ! -f go.mod ]; then
  echo "Error: this is not a go module, is it?"
  exit 1
fi

if [ -z "${CODEGEN_PKG:-}" ]; then
  if [ -d ./vendor/k8s.io/code-generator ]; then
    CODEGEN_PKG=./vendor/k8s.io/code-generator
  else
    CODEGEN_PKG=$(go mod download -json k8s.io/code-generator | jq -r '.Dir //empty')
  fi
fi

if [ -z "${GEN_PKG_PATH:-}" ]; then
  GEN_PKG_PATH=$(go list -m)/pkg
fi

echo "PWD: ${PWD}"
echo "CODEGEN_PKG: ${CODEGEN_PKG}"
echo "GEN_PKG_PATH: ${GEN_PKG_PATH}"

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")
#CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./tools/vendor/k8s.io/code-generator 2>/dev/null)}

#source "${SCRIPT_ROOT}/${CODEGEN_PKG}/kube_codegen.sh"
source "${CODEGEN_PKG}/kube_codegen.sh"

kube::codegen::gen_helpers \
  --boilerplate "./hack/boilerplate.go.txt" \
  "${BASH_SOURCE[0]}"/pkg/api

kube::codegen::gen_client \
    --with-watch \
    --with-applyconfig \
    --output-dir "./pkg/client" \
    --output-pkg "${GEN_PKG_PATH}"/client \
    --boilerplate "./hack//boilerplate.go.txt" \
    ./pkg
