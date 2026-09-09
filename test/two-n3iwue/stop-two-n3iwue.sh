#!/usr/bin/env bash
set -euo pipefail

if ((EUID != 0)); then
  echo "Run as root: sudo $0" >&2
  exit 1
fi
TEST_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
STATE_DIR=$TEST_DIR/state

for ue in 1 2; do
  pidfile=$STATE_DIR/ue$ue.pid
  startfile=$STATE_DIR/ue$ue.starttime
  [[ -s $pidfile && -s $startfile ]] || continue
  pid=$(cat "$pidfile")
  recorded_start=$(cat "$startfile")
  current_start=$(awk '{print $22}' "/proc/$pid/stat" 2>/dev/null || true)
  if [[ $current_start == "$recorded_start" ]]; then
    kill -TERM "$pid" 2>/dev/null || true
    for _ in {1..30}; do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
      kill -KILL "$pid" 2>/dev/null || true
    fi
  elif [[ -n $current_start ]]; then
    echo "Not stopping reused PID $pid for UE$ue" >&2
  fi
  rm -f -- "$pidfile" "$startfile"
done

for ns in n3iwue1 n3iwue2; do
  marker=$STATE_DIR/$ns.created
  if [[ -f $marker ]]; then
    if [[ -e /var/run/netns/$ns ]] && \
       [[ $(stat -Lc '%i' "/var/run/netns/$ns") == "$(cat "$marker")" ]]; then
      ip netns del "$ns"
      rm -f -- "$marker"
    elif [[ ! -e /var/run/netns/$ns ]]; then
      rm -f -- "$marker"
    else
      echo "Not deleting replaced/unowned namespace $ns" >&2
    fi
  fi
done
echo "Stopped owned processes and removed owned namespaces; configs and logs retained."
