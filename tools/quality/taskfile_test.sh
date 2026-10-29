#!/bin/bash
# Exercise dependency and release orchestration in a temporary directory. Git, Go and
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
go_env_log="${release_test_dir}/go-env.log"
fail_tidy_module=""
stale_module=""
fail_get=false
example_modules=(examples/new/deep\ demo examples/relationships examples/relationships-gql examples/rpc/web_debug examples/web)
touch go.mod
for module in gql rpc "${example_modules[@]}"; do
    mkdir -p "${module}"
    touch "${module}/go.mod" "${module}/go.sum"
done
for ignored in examples/.cache/module examples/vendor/module examples/node_modules/module; do
    mkdir -p "${ignored}"
    touch "${ignored}/go.mod"
done
# A module without checksums is valid; release must not try to add a missing file.
rm examples/web/go.sum

function go {
    local module="${PWD#"${release_test_dir}/"}"
    if [ "${PWD}" = "${release_test_dir}" ]; then
        module=.
    fi
    printf '%s|%s\n' "${module}" "$*" >> "${go_log}"
    printf '%s|%s|%s\n' "${module}" "${GOWORK:-}" "${GOTOOLCHAIN:-}" >> "${go_env_log}"
    if [ "$1" = get ] && "${fail_get}"; then
        return 1
    fi
    if [ "${module}" = "${fail_tidy_module}" ] && [ "$*" = "mod tidy" ]; then
        return 1
    fi
    if [ "${module}" = "${stale_module}" ] && [ "$*" = "mod tidy -diff" ]; then
        printf '%s\n' 'diff current/go.mod tidy/go.mod'
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

# Root updates must propagate through libraries before all examples, including
# newly discovered modules and modules excluded from regular lint/test runs.
fail_tidy_module=""
: > "${go_log}"
: > "${git_log}"
: > "${go_env_log}"
GOWORK=/not/a/workspace GO_TOOLCHAIN=local go:deps:update example.com/router@v1.2.3 > update.log 2>&1
{
    printf '%s\n' '.|get example.com/router@v1.2.3'
    for module in . gql rpc "${example_modules[@]}"; do
        printf '%s|mod tidy\n' "${module}"
    done
} > expected.log
diff -u expected.log "${go_log}"
if grep -v '|off|local$' "${go_env_log}"; then
    printf '%s\n' 'FAIL: dependency maintenance used workspace mode or the wrong toolchain' >&2
    exit 1
fi
test ! -s "${git_log}"
printf '%s\n' 'PASS: dependency updates synchronize libraries and every discovered example'

: > "${go_log}"
if go:deps:update > missing-args.log 2>&1; then
    printf '%s\n' 'FAIL: dependency update accepted missing arguments' >&2
    exit 1
fi
test ! -s "${go_log}"
fail_get=true
if go:deps:update example.com/router@v1.2.3 > failed-update.log 2>&1; then
    printf '%s\n' 'FAIL: dependency update ignored go get failure' >&2
    exit 1
fi
printf '%s\n' '.|get example.com/router@v1.2.3' > expected.log
diff -u expected.log "${go_log}"
fail_get=false

: > "${go_log}"
fail_tidy_module=gql
if go:deps:update example.com/router@v1.2.3 > failed-sync.log 2>&1; then
    printf '%s\n' 'FAIL: dependency update ignored library tidy failure' >&2
    exit 1
fi
printf '%s\n' '.|get example.com/router@v1.2.3' '.|mod tidy' 'gql|mod tidy' > expected.log
diff -u expected.log "${go_log}"
grep -Fq 'Dependency tidy failed in gql.' failed-sync.log
fail_tidy_module=""
printf '%s\n' 'PASS: dependency failures stop synchronization and report the affected module'

: > "${go_log}"
go:deps:check > check.log 2>&1
for module in . gql rpc "${example_modules[@]}"; do
    printf '%s|mod tidy -diff\n' "${module}"
done > expected.log
diff -u expected.log "${go_log}"

# Every gate must reject a stale generated example before invoking tests or
# other expensive checks, including gates given an explicit package argument.
function go:fmt:check { :; }
function go:test { printf '%s\n' 'test invoked' >> "${go_log}"; }
stale_module=examples/relationships-gql
for gate in go:quality:pr go:quality:all go:quality:baseline go:quality:ci; do
    : > "${go_log}"
    if "${gate}" ./... > gate.log 2>&1; then
        printf 'FAIL: %s accepted stale example dependencies\n' "${gate}" >&2
        exit 1
    fi
    grep -Fq "Dependency tidy failed in ${stale_module}." gate.log
    grep -Fq 'bash taskfile go:deps:sync' gate.log
    if grep -Ev '\|mod tidy -diff$' "${go_log}"; then
        printf 'FAIL: %s wrote module files or proceeded past dependency checks\n' "${gate}" >&2
        exit 1
    fi
done
test ! -s "${git_log}"
printf '%s\n' 'PASS: every quality gate checks all modules without rewriting dependency files'
