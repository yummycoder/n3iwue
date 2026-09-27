#!/usr/bin/env bash
# A' tunnel-only single-round IKE_AUTH — n3iwue side.
# Run from the n3iwue repo root (branch feat/mn-anchored-sn, with config fields
# TunnelOnly/ControlSocket/Kn3iwfHex already added). Idempotent-ish: re-running
# after success will fail the assert (anchor already changed) — that's expected.
set -euo pipefail
python3 - <<'PY'
import sys

def edit(path, subs):
    s = open(path).read()
    for old, new, label in subs:
        n = s.count(old)
        if n != 1:
            sys.exit(f"[{path}] anchor '{label}' matched {n} times (want 1) — aborting, no changes written")
        s = s.replace(old, new)
    open(path, "w").write(s)
    print(f"[{path}] OK")

# ---------- pkg/ike/send.go ----------
send = "pkg/ike/send.go"
subs_send = [
    (
        '\t"encoding/binary"\n\t"math/big"\n',
        '\t"encoding/binary"\n\t"encoding/hex"\n\t"math/big"\n',
        "import encoding/hex",
    ),
    (
        '\t// Identification\n\tikePayload.BuildIdentificationInitiator(ike_message.ID_KEY_ID, []byte("UE"))\n',
        '\t// Identification. MN-anchored tunnel-only asserts the SUPI so the N3IWF\n'
        '\t// can correlate the AMF-delivered K_N3IWF_SN; stock stays "UE".\n'
        '\tidType := ike_message.ID_KEY_ID\n'
        '\tidData := []byte("UE")\n'
        '\tif factory.N3ueInfo.TunnelOnly {\n'
        '\t\tidData = []byte(factory.N3ueInfo.GetSUPI())\n'
        '\t}\n'
        '\tikePayload.BuildIdentificationInitiator(idType, idData)\n',
        "IDi -> SUPI",
    ),
    (
        '\t\t[]byte{255, 255, 255, 255},\n\t)\n\n\tikeMessage := ike_message.NewMessage(\n'
        '\t\tikeSA.LocalSPI, ikeSA.RemoteSPI,',
        '\t\t[]byte{255, 255, 255, 255},\n\t)\n\n'
        '\tif factory.N3ueInfo.TunnelOnly {\n'
        '\t\t// MN-anchored single-round IKE_AUTH: K_N3IWF_SN is delivered\n'
        '\t\t// out-of-band (Kn3iwfHex in the isolated test, or free-ran-ue over the\n'
        '\t\t// control socket) - NOT derived here, since without EAP there is no\n'
        '\t\t// network RAND to reproduce K_AMF. Fold the SUPI IDi into the signed\n'
        '\t\t// octets and build AUTH keyed by the delivered key; per-tunnel\n'
        '\t\t// freshness is the IKE_SA_INIT nonce/DH exchange (RFC 7296).\n'
        '\t\tif len(n3ueContext.Kn3iwf) != 32 && factory.N3ueInfo.Kn3iwfHex != "" {\n'
        '\t\t\tk, decErr := hex.DecodeString(factory.N3ueInfo.Kn3iwfHex)\n'
        '\t\t\tif decErr != nil || len(k) != 32 {\n'
        '\t\t\t\tikeLog.Errorf("SendIkeAuth: bad Kn3iwfHex")\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\tn3ueContext.Kn3iwf = k\n'
        '\t\t}\n'
        '\t\tif len(n3ueContext.Kn3iwf) != 32 {\n'
        '\t\t\tikeLog.Errorf("SendIkeAuth tunnel-only: no K_N3IWF_SN delivered")\n'
        '\t\t\treturn\n'
        '\t\t}\n\n'
        '\t\tvar idp ike_message.IKEPayloadContainer\n'
        '\t\tidp.BuildIdentificationInitiator(idType, idData)\n'
        '\t\tidpData, encErr := idp.Encode()\n'
        '\t\tif encErr != nil {\n'
        '\t\t\tikeLog.Errorf("SendIkeAuth: encode ID: %v", encErr)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tif _, wErr := ikeSA.Prf_i.Write(idpData[4:]); wErr != nil {\n'
        '\t\t\tikeLog.Errorf("SendIkeAuth: prf_i write: %v", wErr)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tikeSA.ResponderSignedOctets = append(ikeSA.ResponderSignedOctets, ikeSA.Prf_i.Sum(nil)...)\n\n'
        '\t\tprfFn := ikeSA.PrfInfo.Init(n3ueContext.Kn3iwf)\n'
        '\t\tif _, wErr := prfFn.Write([]byte("Key Pad for IKEv2")); wErr != nil {\n'
        '\t\t\tikeLog.Errorf("SendIkeAuth: prf write: %v", wErr)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tsecret := prfFn.Sum(nil)\n'
        '\t\tprfFn = ikeSA.PrfInfo.Init(secret)\n'
        '\t\tprfFn.Reset()\n'
        '\t\tif _, wErr := prfFn.Write(ikeSA.ResponderSignedOctets); wErr != nil {\n'
        '\t\t\tikeLog.Errorf("SendIkeAuth: prf write: %v", wErr)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tikePayload.BuildAuthentication(ike_message.SharedKeyMesageIntegrityCode, prfFn.Sum(nil))\n\n'
        '\t\tcfg := ikePayload.BuildConfiguration(ike_message.CFG_REQUEST)\n'
        '\t\tcfg.ConfigurationAttribute.BuildConfigurationAttribute(ike_message.INTERNAL_IP4_ADDRESS, nil)\n'
        '\t}\n\n'
        '\tikeMessage := ike_message.NewMessage(\n'
        '\t\tikeSA.LocalSPI, ikeSA.RemoteSPI,',
        "insert AUTH+CFG before NewMessage",
    ),
]
edit(send, subs_send)

# ---------- pkg/ike/handler.go ----------
handler = "pkg/ike/handler.go"
subs_handler = [
    (
        '\ts.stopContinuousIkeSaInit()\n\n\ts.SendIkeAuth()\n',
        '\ts.stopContinuousIkeSaInit()\n\n'
        '\tif factory.N3ueInfo.TunnelOnly {\n'
        '\t\t// MN-anchored single-round IKE_AUTH: skip the EAP states so the\n'
        '\t\t// N3IWF\'s sole IKE_AUTH response is processed as the child SA.\n'
        '\t\tikeSecurityAssociation.State = IKEAUTH_Authentication\n'
        '\t}\n\n'
        '\ts.SendIkeAuth()\n',
        "State=IKEAUTH_Authentication in tunnel-only",
    ),
]
edit(handler, subs_handler)
PY
echo "gofmt..."
gofmt -w pkg/ike/send.go pkg/ike/handler.go
echo "build..."
go build ./... && echo "BUILD OK"
