package context

import (
	"net"
	"runtime"
	"testing"

	"github.com/free5gc/n3iwue/internal/qos"
)

func TestPDUSessionWaitsForUserPlane(t *testing.T) {
	session := NewPDUSession(11)
	done := make(chan struct{})
	go func() {
		name, upIP, gotQoS, err := session.WaitUserPlaneReady()
		if err != nil || name != "ipsec-3" || !upIP.Equal(net.IPv4(10, 0, 0, 1)) ||
			gotQoS == nil || gotQoS.PduSessionID != 11 {
			t.Errorf("unexpected ready state: name=%q ip=%v qos=%+v err=%v", name, upIP, gotQoS, err)
		}
		close(done)
	}()

	runtime.Gosched()
	select {
	case <-done:
		t.Fatal("WaitUserPlaneReady returned before XFRM setup")
	default:
	}

	session.SetUserPlane(net.IPv4(10, 0, 0, 1), &qos.PDUQoSInfo{PduSessionID: 11}, nil, "ipsec-3")
	session.MarkUserPlaneReady(nil)
	<-done
}

func TestDeletePDUSessionAllowsIDReuse(t *testing.T) {
	n3ue := &N3UE{PduSessions: make(map[uint8]*PDUSession)}
	first := n3ue.GetOrCreatePDUSession(10)
	n3ue.DeletePDUSession(10)
	second := n3ue.GetOrCreatePDUSession(10)
	if first == second {
		t.Fatal("recreated PDU session reused released mutable state")
	}
}
