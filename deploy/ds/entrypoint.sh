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

if [[ -n "${DS_SUFFIX:-}" ]]; then
    if wait_healthy; then
        ensure_suffix "$DS_SUFFIX" || log "WARNING: failed to create suffix $DS_SUFFIX"
    else
        log "WARNING: server did not become healthy, skipping suffix creation"
    fi
fi

# Wait for dscontainer; `wait` is interrupted by trapped signals, so loop until it is gone.
rc=0
while kill -0 "$pid" 2>/dev/null; do
    wait "$pid" && rc=0 || rc=$?
done
exit "$rc"
