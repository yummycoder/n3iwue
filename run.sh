#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
BINARY=$SCRIPT_DIR/n3iwue
CONFIG_FILE=${1:-$SCRIPT_DIR/config/n3ue.yaml}
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Configuration file not found: $CONFIG_FILE" >&2
    exit 1
fi
CONFIG_FILE=$(readlink -f "$CONFIG_FILE")
if [[ ! -x "$BINARY" ]]; then
    echo "N3IWUE binary not found: $BINARY" >&2
    echo "Build it with: cd $SCRIPT_DIR && CGO_ENABLED=0 go build -o n3iwue ./n3iwue.go" >&2
    exit 1
fi
if find "$SCRIPT_DIR" -path "$SCRIPT_DIR/.git" -prune -o -name '*.go' -newer "$BINARY" -print -quit | grep -q .; then
    echo "N3IWUE binary is older than the Go source and may not support -config." >&2
    echo "Rebuild it with: cd $SCRIPT_DIR && CGO_ENABLED=0 go build -o n3iwue ./n3iwue.go" >&2
    exit 1
fi

sudo -v

PID_LIST=()

function terminate()
{
    sleep 1
    sudo "$SCRIPT_DIR/clean.sh"

    if ((${#PID_LIST[@]})); then
        sudo kill -SIGTERM "${PID_LIST[@]}" 2>/dev/null || true
    fi
}

# Run N3UE without packet capture. Keep stdout/stderr on the terminal.
cd "$SCRIPT_DIR"
sudo "$BINARY" -config "$CONFIG_FILE" &
SUDO_N3UE_PID=$!
sleep 0.1
echo SUDO_N3UE_PID $SUDO_N3UE_PID
N3UE_PID=$(pgrep -P "${SUDO_N3UE_PID}")
PID_LIST+=("$SUDO_N3UE_PID" "$N3UE_PID")
echo N3UE_PID ${N3UE_PID}

trap terminate SIGINT
wait ${PID_LIST}
exit 0
