# Two simultaneous N3IWUE instances

This test places each N3IWUE process, its XFRM state, routes, VRFs, and generated
interfaces in a separate network namespace. It never runs either UE in the root
namespace. The setup defaults to the interface named by the current working
configuration (`enp7s0`), but still refuses it if it carries the default route or
the current SSH connection.

The checked-in `*.example.yaml` configurations are deliberately redacted. Copy
them to the ignored local config names, then replace every `CHANGE_ME_*` value
with two different, provisioned subscribers. Keep secrets out of command lines,
Git, and logs.
Confirm both SUPIs are provisioned in the core. UE1 uses `192.168.2.1` and
`02:00:00:00:02:01`; UE2 uses `192.168.2.3` and `02:00:00:00:02:03`. Both request
one PDU session (ID 10), DNN `internet`, and S-NSSAI `1/010203`.

## Build and prepare

These are commands for the operator to run; the implementation did not launch
the simulation.

```bash
cd /home/ubuntu/n3iwue
cp test/two-n3iwue/config/ue1.example.yaml test/two-n3iwue/config/ue1.yaml
cp test/two-n3iwue/config/ue2.example.yaml test/two-n3iwue/config/ue2.yaml
# Edit the two local files and replace every CHANGE_ME_* value.

CGO_ENABLED=0 go build -o n3iwue-two-ue ./n3iwue.go
go test ./...

# Review redacted, non-secret differences without printing credential values:
diff -u \
  <(sed -E '/^[[:space:]]+(K|RAND|SQN|AMF|OP|OPC):/s/:.*/: [REDACTED]/' test/two-n3iwue/config/ue1.yaml) \
  <(sed -E '/^[[:space:]]+(K|RAND|SQN|AMF|OP|OPC):/s/:.*/: [REDACTED]/' test/two-n3iwue/config/ue2.yaml)

sudo test/two-n3iwue/setup-two-n3iwue.sh enp7s0
sudo test/two-n3iwue/run-two-n3iwue.sh
sudo test/two-n3iwue/status-two-n3iwue.sh
```

The single-instance launcher now also accepts a configuration path:

```bash
sudo ./run.sh /absolute/path/to/ue.yaml
# Equivalent direct invocation:
sudo ./n3iwue -config /absolute/path/to/ue.yaml
```

Runtime files are under `test/two-n3iwue/runtime`, PID ownership records under
`state`, and timestamped output under `logs`. The application updates SQN in the
specific per-UE config it was launched with, so use writable copies and retain
them between runs.

## Validate traffic

Wait until status shows a registration accept and `PduSession Created` for both.
The expected PDU device is `gretun-pdu-10` and VRF is `vrf-pdu-10` inside each
namespace. Discover the assigned addresses, then ping the DN:

```bash
UE1_PDU_IP=$(sudo ip -n n3iwue1 -4 -o addr show dev gretun-pdu-10 | awk '{sub(/\/.*/, "", $4); print $4}')
UE2_PDU_IP=$(sudo ip -n n3iwue2 -4 -o addr show dev gretun-pdu-10 | awk '{sub(/\/.*/, "", $4); print $4}')
test -n "$UE1_PDU_IP" && test -n "$UE2_PDU_IP" && test "$UE1_PDU_IP" != "$UE2_PDU_IP"
sudo ip netns exec n3iwue1 ip vrf exec vrf-pdu-10 ping -c 5 -I "$UE1_PDU_IP" 192.168.3.2
sudo ip netns exec n3iwue2 ip vrf exec vrf-pdu-10 ping -c 5 -I "$UE2_PDU_IP" 192.168.3.2
```

Start DN-side iperf3 servers on ports 5201 and 5202. Then run the two UE flows
simultaneously (add `-R` to both client commands for the reverse direction):

```bash
sudo ip netns exec n3iwue1 ip vrf exec vrf-pdu-10 iperf3 -B "$UE1_PDU_IP" -c 192.168.3.2 -p 5201 -t 120 &
UE1_IPERF_PID=$!
sudo ip netns exec n3iwue2 ip vrf exec vrf-pdu-10 iperf3 -B "$UE2_PDU_IP" -c 192.168.3.2 -p 5202 -t 120 &
UE2_IPERF_PID=$!
wait "$UE1_IPERF_PID" "$UE2_IPERF_PID"
```

For repeatable JSON-producing trials, create a result directory outside the
repository and use the included runner:

```bash
mkdir -p /tmp/n3iwue-two-ue-results/trial-1
test/two-n3iwue/run-iperf.sh \
  bidir both /tmp/n3iwue-two-ue-results/trial-1 120 5
```

Capture the physical NWu link while testing, then verify ESP/NAT-T is present
and clear GRE is absent:

```bash
sudo tcpdump -ni enp7s0 -w test/two-n3iwue/logs/two-ue-nwu.pcap \
  'host 192.168.2.2 and (esp or udp port 4500 or udp port 500 or gre)'
tcpdump -nr test/two-n3iwue/logs/two-ue-nwu.pcap 'esp or udp port 4500'
tcpdump -nr test/two-n3iwue/logs/two-ue-nwu.pcap gre
```

The final GRE command should print no packets. Compare the two namespaces'
redacted XFRM output in the status report to confirm independent SPIs, and use
the N3IWF/SMF logs to confirm distinct SUPIs, PDU addresses, and TEIDs.

## Stop and clean up

```bash
sudo test/two-n3iwue/stop-two-n3iwue.sh
```

Cleanup signals only recorded, start-time-matched PIDs and deletes only
namespaces carrying this test's ownership markers. Logs and configs remain.
