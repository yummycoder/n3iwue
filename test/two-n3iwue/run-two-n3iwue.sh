#!/usr/bin/env bash
set -euo pipefail

if ((EUID != 0)); then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

TEST_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_DIR=$(cd -- "$TEST_DIR/../.." && pwd)
STATE_DIR=$TEST_DIR/state
LOG_DIR=$TEST_DIR/logs
BINARY=${N3IWUE_BINARY:-$REPO_DIR/n3iwue-two-ue}
mkdir -p "$STATE_DIR" "$LOG_DIR" "$TEST_DIR/runtime/ue1" "$TEST_DIR/runtime/ue2"

[[ -x $BINARY ]] || { echo "Missing binary: $BINARY (build it using the README command)" >&2; exit 1; }

for ue in 1 2; do
  ns=n3iwue$ue
  cfg=$TEST_DIR/config/ue$ue.yaml
  ip netns list | awk '{print $1}' | grep -Fxq "$ns" || { echo "Missing namespace $ns; run setup first" >&2; exit 1; }
  [[ -f $cfg ]] || { echo "Missing config $cfg" >&2; exit 1; }
  if grep -q 'CHANGE_ME_' "$cfg"; then
    echo "Config ue$ue.yaml still contains credential placeholders" >&2
    exit 1
  fi
  expected_addr=192.168.2.$((ue == 1 ? 1 : 3))
  configured_addr=$(awk '/IPSecIfaceAddr:/{value=$2} END{print value}' "$cfg")
  [[ $configured_addr == "$expected_addr" ]] || {
    echo "UE$ue config outer address must be $expected_addr (got $configured_addr)" >&2
    exit 1
  }
  if [[ -s $STATE_DIR/ue$ue.pid ]] && kill -0 "$(cat "$STATE_DIR/ue$ue.pid")" 2>/dev/null; then
    echo "UE$ue is already running (PID $(cat "$STATE_DIR/ue$ue.pid"))" >&2
    exit 1
  fi
done

ue1_msin=$(awk '/MSIN:/{gsub(/["[:space:]]/,"",$2); print $2; exit}' "$TEST_DIR/config/ue1.yaml")
ue2_msin=$(awk '/MSIN:/{gsub(/["[:space:]]/,"",$2); print $2; exit}' "$TEST_DIR/config/ue2.yaml")
[[ -n $ue1_msin && -n $ue2_msin && $ue1_msin != "$ue2_msin" ]] || { echo "UE configs must have different MSINs" >&2; exit 1; }

stamp=$(date -u +%Y%m%dT%H%M%SZ)
start_ue() {
  local ue=$1 ns=n3iwue$1 cfg=$TEST_DIR/config/ue$1.yaml runtime=$TEST_DIR/runtime/ue$1
  local log=$LOG_DIR/ue$1-$stamp.log
  (
    cd "$runtime"
    exec ip netns exec "$ns" "$BINARY" -config "$cfg"
  ) >"$log" 2>&1 &
  local pid=$!
  printf '%s\n' "$pid" >"$STATE_DIR/ue$ue.pid"
  awk '{print $22}' "/proc/$pid/stat" >"$STATE_DIR/ue$ue.starttime"
  printf '%s\n' "$log" >"$STATE_DIR/ue$ue.log"
}

start_ue 1
start_ue 2
sleep 3
for ue in 1 2; do
  pid=$(cat "$STATE_DIR/ue$ue.pid")
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "UE$ue exited during startup; see $(cat "$STATE_DIR/ue$ue.log")" >&2
    "$TEST_DIR/stop-two-n3iwue.sh" || true
    exit 1
  fi
done

echo "Both N3IWUE processes started. Logs:"
echo "  tail -F $(cat "$STATE_DIR/ue1.log")"
echo "  tail -F $(cat "$STATE_DIR/ue2.log")"
