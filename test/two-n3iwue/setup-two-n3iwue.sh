#!/usr/bin/env bash
set -euo pipefail

if ((EUID != 0)); then
  echo "Run as root: sudo $0 <dedicated-NWu-interface>" >&2
  exit 1
fi

PARENT_IF=${1:-${NWU_PARENT_IF:-enp7s0}}
STATE_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/state
mkdir -p "$STATE_DIR"

command -v ip >/dev/null || { echo "iproute2 is required" >&2; exit 1; }
ip link show dev "$PARENT_IF" >/dev/null 2>&1 || { echo "Interface $PARENT_IF does not exist" >&2; exit 1; }

default_if=$(ip -4 route show default | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i=="dev") print $(i+1)}')
if [[ $default_if == "$PARENT_IF" ]]; then
  echo "Refusing: $PARENT_IF carries the IPv4 default route" >&2
  exit 1
fi
if [[ -n ${SSH_CONNECTION:-} ]]; then
  read -r ssh_peer _ ssh_local _ <<<"$SSH_CONNECTION"
  for endpoint in "$ssh_peer" "$ssh_local"; do
    route_if=$(ip -4 route get "$endpoint" 2>/dev/null | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i=="dev") print $(i+1)}')
    if [[ $route_if == "$PARENT_IF" ]]; then
      echo "Refusing: $PARENT_IF carries this SSH connection" >&2
      exit 1
    fi
  done
fi
if ip -4 addr show dev "$PARENT_IF" | grep -Eq 'inet 192\.168\.2\.(1|3)/'; then
  echo "Refusing: the parent must not own either UE address" >&2
  exit 1
fi

for ns in n3iwue1 n3iwue2; do
  if ip netns list | awk '{print $1}' | grep -Fxq "$ns"; then
    marker=$STATE_DIR/$ns.created
    inode=$(stat -Lc '%i' "/var/run/netns/$ns")
    if [[ ! -f $marker || $(cat "$marker") != "$inode" ]]; then
      echo "Refusing to reuse unowned namespace $ns" >&2
      exit 1
    fi
  fi
done

setup_ue() {
  local ns=$1 temp_if=$2 mac=$3 addr=$4
  if ! ip netns list | awk '{print $1}' | grep -Fxq "$ns"; then
    ip netns add "$ns"
    stat -Lc '%i' "/var/run/netns/$ns" >"$STATE_DIR/$ns.created"
  fi
  ip -n "$ns" link set lo up
  if ! ip -n "$ns" link show enp7s0 >/dev/null 2>&1; then
    ip link show "$temp_if" >/dev/null 2>&1 && { echo "Refusing to reuse unowned link $temp_if" >&2; exit 1; }
    ip link add "$temp_if" link "$PARENT_IF" address "$mac" type macvlan mode bridge
    ip link set "$temp_if" netns "$ns"
    ip -n "$ns" link set "$temp_if" name enp7s0
  fi
  actual_mac=$(ip -n "$ns" -brief link show enp7s0 | awk '{print $3}')
  [[ $actual_mac == "$mac" ]] || { echo "$ns has unexpected MAC $actual_mac" >&2; exit 1; }
  ip -n "$ns" addr replace "$addr/24" dev enp7s0
  ip -n "$ns" link set enp7s0 up
}

setup_ue n3iwue1 n3iwue1-mv 02:00:00:00:02:01 192.168.2.1
setup_ue n3iwue2 n3iwue2-mv 02:00:00:00:02:03 192.168.2.3

[[ $(ip -n n3iwue1 link show enp7s0 | awk '/link\/ether/{print $2}') != \
   $(ip -n n3iwue2 link show enp7s0 | awk '/link\/ether/{print $2}') ]] || { echo "Duplicate MAC addresses" >&2; exit 1; }
[[ $(ip -n n3iwue1 -4 -o addr show enp7s0 | awk '{print $4}') != \
   $(ip -n n3iwue2 -4 -o addr show enp7s0 | awk '{print $4}') ]] || { echo "Duplicate IP addresses" >&2; exit 1; }

printf '%-10s %-10s %-20s %s\n' NAMESPACE INTERFACE MAC ADDRESS
for ns in n3iwue1 n3iwue2; do
  mac=$(ip -n "$ns" link show enp7s0 | awk '/link\/ether/{print $2}')
  addr=$(ip -n "$ns" -4 -o addr show enp7s0 | awk '{print $4}')
  printf '%-10s %-10s %-20s %s\n' "$ns" enp7s0 "$mac" "$addr"
done
