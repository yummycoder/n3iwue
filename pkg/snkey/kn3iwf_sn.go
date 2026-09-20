// Package snkey derives the N3IWF secondary-leg key K_N3IWF_SN for the
// MN-anchored NR-DC model, where the UE performs a single 3GPP registration
// via the master node and no EAP-5G exchange occurs over the N3IWF.
//
// K_AMF is established independently on the UE and the AMF during the primary
// 5G-AKA authentication (TS 33.501 clause 6). Both endpoints therefore already
// share it without transmission. K_N3IWF_SN is derived from K_AMF using the
// TS 33.220 B.2.0 KDF (HMAC-SHA-256), reusing the assigned FC for
// K_gNB/K_N3IWF derivation (0x6E), but with a *deterministic* parameter set
// drawn entirely from context both endpoints already hold:
//
//	P0 = SUPI            (identity binding)
//	P1 = access-type     (non-3GPP distinguisher, as in TS 33.501 A.9)
//	P2 = PDU session ID  (binds the key to the session being split)
//
// No uplink NAS COUNT is used: the standard A.9 derivation mixes in ULCount for
// freshness, which would require the UE and AMF to agree on a COUNT value at
// add time. That is deliberately avoided here. Per-tunnel key freshness is
// instead provided by the IKE_SA_INIT exchange (initiator/responder nonces and
// the Diffie-Hellman shared secret feed SKEYSEED per RFC 7296 section 2.14);
// K_N3IWF_SN serves only as the shared secret authenticating IKE_AUTH
// ("Key Pad for IKEv2", RFC 7296 section 2.15). A deterministic K_N3IWF_SN is
// therefore safe: the child-SA keys still differ on every establishment.
package snkey

import (
	"github.com/free5gc/util/ueauth"
)

// AccessTypeNon3GPP is the non-3GPP access-type distinguisher octet
// (TS 33.501 table A.9-1). Kept as a package constant so the UE and AMF sides
// cannot drift.
const AccessTypeNon3GPP uint8 = 0x02

// DeriveKN3IWFSN computes the MN-anchored secondary-leg N3IWF key.
//
// kamf is the raw 32-byte K_AMF (NOT hex). supi is the UE SUPI exactly as held
// in the security context (e.g. "imsi-208930000000001"). pduSessionID is the
// 5GSM PDU session identifier of the session whose downlink is being split.
//
// The result is a 32-byte key identical on the UE and AMF when both pass the
// same (kamf, supi, pduSessionID). It is byte-for-byte reproducible because it
// uses free5gc's canonical GetKDFValue with fixed-order parameters and lengths.
func DeriveKN3IWFSN(kamf []byte, supi string, pduSessionID uint8) ([]byte, error) {
	p0 := []byte(supi)
	l0 := ueauth.KDFLen(p0)

	p1 := []byte{AccessTypeNon3GPP}
	l1 := ueauth.KDFLen(p1)

	p2 := []byte{pduSessionID}
	l2 := ueauth.KDFLen(p2)

	return ueauth.GetKDFValue(
		kamf,
		ueauth.FC_FOR_KGNB_KN3IWF_DERIVATION, // "6E"
		p0, l0,
		p1, l1,
		p2, l2,
	)
}
