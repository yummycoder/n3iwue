package service

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/free5gc/n3iwue/internal/logger"
	n3iwue_context "github.com/free5gc/n3iwue/pkg/context"
	"github.com/free5gc/n3iwue/pkg/factory"
)

// snKeyOnce ensures a single IKE SA establishment per process, even if
// free-ran-ue delivers K_N3IWF_SN more than once (e.g. SIGUSR1 reaching both
// the sudo parent and the UE child). A second IKE_SA_INIT would clobber the
// single IKE SA context and corrupt the SK_e/SK_a keys.
var snKeyOnce sync.Once

// snKeyMsg is the control-plane message free-ran-ue sends over the SN key
// channel: one 32-byte K_N3IWF_SN per IKE SA setup. NOT a data-plane path.
type snKeyMsg struct {
	Supi         string `json:"supi"`
	PduSessionID uint8  `json:"pduSessionID"`
	Kn3iwfHex    string `json:"kN3iwfHex"`
}

func snKeyChannelAddr() (network, addr string) {
	addr = factory.N3ueInfo.SNKeyChannel
	if addr == "" {
		addr = "/run/l25gc/n3iwf-ue-key.sock"
	}
	if strings.HasPrefix(addr, "/") {
		return "unix", addr
	}
	return "tcp", addr
}

// listenSNKeyChannel receives K_N3IWF_SN from the co-located free-ran-ue and
// starts IKE. Active only in MN-anchored socket-keyed mode (TunnelOnly, no
// static Kn3iwfHex). Control-plane only; never touches the IPsec data path.
func (a *N3iwueApp) listenSNKeyChannel() {
	defer a.wg.Done()
	log := logger.InitLog

	if !factory.N3ueInfo.TunnelOnly || factory.N3ueInfo.Kn3iwfHex != "" {
		return
	}

	network, addr := snKeyChannelAddr()
	if network == "unix" {
		_ = os.Remove(addr)
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		log.Errorf("SN key channel: listen %s %s failed: %v", network, addr, err)
		return
	}
	defer ln.Close()
	log.Infof("SN key channel: listening on %s %s (awaiting K_N3IWF_SN from free-ran-ue)", network, addr)

	go func() {
		<-a.ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-a.ctx.Done():
				return
			default:
				log.Warnf("SN key channel: accept: %v", err)
				continue
			}
		}
		a.handleSNKeyConn(conn)
	}
}

func (a *N3iwueApp) handleSNKeyConn(conn net.Conn) {
	defer conn.Close()
	log := logger.InitLog

	var m snKeyMsg
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&m); err != nil {
		log.Errorf("SN key channel: decode: %v", err)
		return
	}
	key, err := hex.DecodeString(m.Kn3iwfHex)
	if err != nil || len(key) != 32 {
		log.Errorf("SN key channel: invalid key (len=%d err=%v)", len(key), err)
		return
	}

	fired := false
	snKeyOnce.Do(func() {
		ctx := a.Context()
		ctx.Kn3iwf = key
		ctx.PendingPduSessionID = m.PduSessionID
		log.Infof("SN key channel: received K_N3IWF_SN SUPI=%s PDU=%d key=%x -> starting IKE SA establishment",
			m.Supi, m.PduSessionID, key)
		a.SendIkeEvt(n3iwue_context.NewStartIkeSaEstablishmentEvt())
		fired = true
	})
	if !fired {
		log.Infof("SN key channel: duplicate K_N3IWF_SN for SUPI=%s ignored (IKE already triggered)", m.Supi)
	}
}
