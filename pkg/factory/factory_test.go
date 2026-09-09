package factory

import "testing"

func TestInitializeRejectsEmptyConfigPath(t *testing.T) {
	if err := Initialize(""); err == nil {
		t.Fatal("Initialize accepted an empty config path")
	}
}

func TestRequestedPDUSessionIDsValidation(t *testing.T) {
	info := N3UEInfo{RequestedPDUSessionIDs: []uint8{10, 10}}
	if _, err := info.validate(); err == nil {
		t.Fatal("duplicate requested PDU session IDs were accepted")
	}
	info.RequestedPDUSessionIDs = []uint8{0}
	if _, err := info.validate(); err == nil {
		t.Fatal("out-of-range requested PDU session ID was accepted")
	}
}
