#!/bin/bash
# Wrapper around upstream dscontainer:
#  * DS_DM_PASSWORD_FILE  -> DS_DM_PASSWORD (dscontainer applies it on every start)
#  * DS_SUFFIX            -> creates backend + root entry on first start if missing
#  * DS_BACKEND           -> backend name for DS_SUFFIX (default: userroot)
set -euo pipefail

log() { echo "ds-entrypoint: $*" >&2; }

LDAPI_URL="ldapi://%2fdata%2frun%2fslapd-localhost.socket"

if [[ -n "${DS_DM_PASSWORD_FILE:-}" ]]; then
    if [[ ! -r "$DS_DM_PASSWORD_FILE" ]]; then
        log "DS_DM_PASSWORD_FILE=$DS_DM_PASSWORD_FILE is not readable"
        exit 1
    fi
    DS_DM_PASSWORD="$(tr -d '\r\n' < "$DS_DM_PASSWORD_FILE")"
    export DS_DM_PASSWORD
fi

/usr/libexec/dirsrv/dscontainer -r &
pid=$!

term() { kill -TERM "$pid" 2>/dev/null || true; }
trap term TERM INT

wait_healthy() {
    local timeout="${DS_STARTUP_TIMEOUT:-60}" i
    for ((i = 0; i < timeout; i += 2)); do
        kill -0 "$pid" 2>/dev/null || return 1
        if /usr/libexec/dirsrv/dscontainer -H >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    return 1
}

ensure_suffix() {
    local suffix="$1" backend="${DS_BACKEND:-userroot}"
    if dsconf "$LDAPI_URL" backend suffix list 2>/dev/null | grep -Fqi "$suffix"; then
        log "suffix $suffix already exists"
        return 0
    fi
    log "creating suffix $suffix (backend $backend)"
    dsconf "$LDAPI_URL" backend create --suffix "$suffix" --be-name "$backend" --create-suffix
}

# On first start dscontainer briefly runs the server while creating the
# instance and then restarts it, so a single health check can hit the
# temporary server. Retry until the suffix exists or the timeout expires.
if [[ -n "${DS_SUFFIX:-}" ]]; then
    deadline=$((SECONDS + ${DS_STARTUP_TIMEOUT:-60} * 2))
    while :; do
        if wait_healthy && ensure_suffix "$DS_SUFFIX"; then
            log "suffix $DS_SUFFIX ready"
            break
        fi
        if ((SECONDS >= deadline)) || ! kill -0 "$pid" 2>/dev/null; then
            log "WARNING: failed to create suffix $DS_SUFFIX"
            break
        fi
        log "server not ready yet, retrying suffix creation"
        sleep 3
    done
fi

# Wait for dscontainer; `wait` is interrupted by trapped signals, so loop until it is gone.
rc=0
while kill -0 "$pid" 2>/dev/null; do
    wait "$pid" && rc=0 || rc=$?
done
exit "$rc"
