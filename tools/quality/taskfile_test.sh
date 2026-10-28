#!/bin/bash
# Exercise release orchestration in a temporary directory. All Git, Go and
# changelog commands are shell mocks: no commits, tags, pushes or installs occur.
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
taskfile_under_test="${TASKFILE_UNDER_TEST:-${repo_dir}/taskfile}"
release_test_dir="$(mktemp -d "${TMPDIR:-/tmp}/go-crud-release-test.XXXXXX")"
trap 'rm -rf "${release_test_dir}"' EXIT
cd "${release_test_dir}"
release_test_dir="$(pwd)"

set -- true
source "${taskfile_under_test}"

go_log="${release_test_dir}/go.log"
git_log="${release_test_dir}/git.log"
fail_tidy_module=""
example_modules=(examples/web examples/relationships examples/relationships-gql examples/rpc/web_debug)
for module in gql rpc "${example_modules[@]}"; do
    mkdir -p "${module}"
    touch "${module}/go.mod" "${module}/go.sum"
done
# A module without checksums is valid; release must not try to add a missing file.
rm examples/web/go.sum

function go {
    local module="${PWD#"${release_test_dir}/"}"
    printf '%s|%s\n' "${module}" "$*" >> "${go_log}"
    if [ "${module}" = "${fail_tidy_module}" ] && [ "$*" = "mod tidy" ]; then
        return 1
    fi
}

function git {
    printf '%s\n' "$*" >> "${git_log}"
    case "$1" in
        rev-parse) return 1 ;; # No existing release tags.
        symbolic-ref) printf '%s\n' main ;;
    esac
}

function git-cliff {
    printf '%s\n' 'Test changelog'
}

printf '%s\n' 0.1.0 > .version
release patch > success.log 2>&1
for module in "${example_modules[@]}"; do
    grep -Fxq "${module}|mod edit -require=github.com/goliatone/go-crud@v0.1.1" "${go_log}"
    grep -Fxq "${module}|mod tidy" "${go_log}"
    grep -Fxq "add ${module}/go.mod" "${git_log}"
    if [ -f "${module}/go.sum" ]; then
        grep -Fxq "add ${module}/go.sum" "${git_log}"
    fi
done
if grep -Fxq 'add examples/web/go.sum' "${git_log}"; then
    printf '%s\n' 'FAIL: release adds a missing go.sum' >&2
    exit 1
fi
grep -Fxq 'commit -m Bump version: v0.1.1' "${git_log}"
printf '%s\n' 'PASS: release synchronizes and includes every example module'

# Dependency failures must stop the release before recording or publishing it.
printf '%s\n' 0.1.0 > .version
: > "${git_log}"
fail_tidy_module=examples/relationships-gql
if release patch > failure.log 2>&1; then
    printf '%s\n' 'FAIL: release ignored an example tidy error' >&2
    exit 1
fi
if grep -Eq '^(commit|tag -a|push) ' "${git_log}"; then
    printf '%s\n' 'FAIL: release proceeded after an example tidy error' >&2
    exit 1
fi
printf '%s\n' 'PASS: example synchronization failure stops release before commit/tag/push'
