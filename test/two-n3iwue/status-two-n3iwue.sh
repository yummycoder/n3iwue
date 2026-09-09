#!/usr/bin/env bash
set -euo pipefail

if ((EUID != 0)); then
  echo "Run as root: sudo $0" >&2
  exit 1
fi
TEST_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
STATE_DIR=$TEST_DIR/state

for ue in 1 2; do
  ns=n3iwue$ue
  echo "===== UE$ue ($ns) ====="
  if [[ -s $STATE_DIR/ue$ue.pid ]]; then
    pid=$(cat "$STATE_DIR/ue$ue.pid")
    recorded_start=$(cat "$STATE_DIR/ue$ue.starttime" 2>/dev/null || true)
    current_start=$(awk '{print $22}' "/proc/$pid/stat" 2>/dev/null || true)
    if [[ -n $recorded_start && $current_start == "$recorded_start" ]]; then
      echo "process: running (PID $pid)"
    else
      echo "process: stopped or PID reused"
    fi
  else
    echo "process: no PID recorded"
  fi
  if ! ip netns list | awk '{print $1}' | grep -Fxq "$ns"; then
    echo "namespace: absent"
    continue
  fi
  echo "-- links and addresses --"
  ip -n "$ns" -brief link
  ip -n "$ns" -brief address
  echo "-- UDP 500/4500 --"
  ip netns exec "$ns" ss -H -lunp '( sport = :500 or sport = :4500 )' || true
  echo "-- XFRM states (key material redacted) --"
  ip netns exec "$ns" ip -s xfrm state | sed -E '/^[[:space:]]+(auth|auth-trunc|aead|enc) /s/(auth|auth-trunc|aead|enc).*/\1 [REDACTED]/'
  echo "-- XFRM policies --"
  ip netns exec "$ns" ip -s xfrm policy
  echo "-- routes --"
  ip -n "$ns" route show table all
  echo "-- recent acceptance messages --"
  if [[ -s $STATE_DIR/ue$ue.log ]]; then
    log=$(cat "$STATE_DIR/ue$ue.log")
    grep -Ei 'Get Registration Accept|PDU session [0-9]+ address|PduSession Created|Setup XFRM interface|Use VRF|bound to VRF|error|fatal' "$log" | tail -n 30 || true
  else
    echo "no log recorded"
  fi
done
