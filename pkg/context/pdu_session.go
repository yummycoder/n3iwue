package context

import (
	"net"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/free5gc/n3iwue/internal/qos"
)

// PDUSession holds mutable user-plane state that must not be shared between
// simultaneous PDU sessions.
type PDUSession struct {
	mu         sync.RWMutex
	ready      chan struct{}
	readyOnce  sync.Once
	readyErr   error
	ID         uint8
	PDUAddress net.IP
	UPIPAddr   net.IP
	QoSInfo    *qos.PDUQoSInfo
	XfrmIface  netlink.Link
	XfrmiName  string
	GRETunnels map[uint8]*netlink.Link
	VRF        netlink.Link
	VRFName    string
	RouteTable int
}

func NewPDUSession(id uint8) *PDUSession {
	return &PDUSession{ID: id, ready: make(chan struct{})}
}

func (n3ue *N3UE) GetOrCreatePDUSession(id uint8) *PDUSession {
	n3ue.PduSessionsMu.Lock()
	defer n3ue.PduSessionsMu.Unlock()
	if session := n3ue.PduSessions[id]; session != nil {
		return session
	}
	session := NewPDUSession(id)
	n3ue.PduSessions[id] = session
	return session
}

func (n3ue *N3UE) GetPDUSession(id uint8) *PDUSession {
	n3ue.PduSessionsMu.RLock()
	defer n3ue.PduSessionsMu.RUnlock()
	return n3ue.PduSessions[id]
}

func (n3ue *N3UE) DeletePDUSession(id uint8) {
	n3ue.PduSessionsMu.Lock()
	defer n3ue.PduSessionsMu.Unlock()
	delete(n3ue.PduSessions, id)
}

func (session *PDUSession) NetworkLinks() ([]netlink.Link, netlink.Link) {
	session.mu.RLock()
	defer session.mu.RUnlock()
	greLinks := make([]netlink.Link, 0, len(session.GRETunnels))
	for _, link := range session.GRETunnels {
		if link != nil && *link != nil {
			greLinks = append(greLinks, *link)
		}
	}
	return greLinks, session.VRF
}

func (session *PDUSession) SetUserPlane(
	upIPAddr net.IP, qosInfo *qos.PDUQoSInfo, xfrmIface netlink.Link, xfrmiName string,
) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.UPIPAddr = append(net.IP(nil), upIPAddr...)
	session.QoSInfo = qosInfo
	session.XfrmIface = xfrmIface
	session.XfrmiName = xfrmiName
}

func (session *PDUSession) MarkUserPlaneReady(err error) {
	session.readyOnce.Do(func() {
		session.mu.Lock()
		session.readyErr = err
		session.mu.Unlock()
		close(session.ready)
	})
}

func (session *PDUSession) WaitUserPlaneReady() (string, net.IP, *qos.PDUQoSInfo, error) {
	<-session.ready
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.XfrmiName, append(net.IP(nil), session.UPIPAddr...), session.QoSInfo, session.readyErr
}
