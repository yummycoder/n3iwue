package nwucp

import (
	"fmt"
	"net"
	"strconv"

	"github.com/vishvananda/netlink"

	"github.com/free5gc/n3iwue/internal/gre"
	"github.com/free5gc/n3iwue/internal/logger"
	"github.com/free5gc/n3iwue/internal/packet/nasPacket"
	"github.com/free5gc/n3iwue/internal/vrf"
	context "github.com/free5gc/n3iwue/pkg/context"
	"github.com/free5gc/n3iwue/pkg/factory"
	"github.com/free5gc/nas"
	"github.com/free5gc/nas/nasMessage"
	"github.com/free5gc/nas/nasType"
	"github.com/free5gc/openapi/models"
)

func (s *Server) handleEvent(evt context.NwucpEvt) {
	switch t := evt.(type) {
	case *context.StartNwucpConnEvt:
		s.handleStartNwucpConnEvt()
	case *context.HandleRegistrationAcceptEvt:
		s.handleRegistrationAccept(t)
	case *context.HandleConfigurationUpdateCommandEvt:
		s.handleConfigurationUpdateCommand(t)
	case *context.HandleDLNASTransportEvt:
		s.handleDLNASTransport(t)
	case *context.StartPduSessionEstablishmentEvt:
		s.handleStartPduSessionEstablishmentEvt()
	case *context.ReleasePduSessionEvt:
		s.handleReleasePduSessionEvt(t.PduSessionID)
	case *context.EstablishPduSessionEvt:
		s.handleEstablishPduSessionEvt(t.PduSessionID)
	case *context.SendDeregistrationEvt:
		s.handleSendDeregistrationEvt()
	case *context.HandleDeregistrationReqUeTerminatedEvt:
		s.handleDeregistrationReqUeTerminated(t)
	}
}

func configurationUpdateNeedsAck(command *nasMessage.ConfigurationUpdateCommand) bool {
	return command != nil && command.ConfigurationUpdateIndication != nil &&
		command.ConfigurationUpdateIndication.GetACK() == 1
}

func (s *Server) handleConfigurationUpdateCommand(evt *context.HandleConfigurationUpdateCommandEvt) {
	if evt == nil || evt.NasMsg == nil || evt.NasMsg.ConfigurationUpdateCommand == nil {
		logger.NWuCPLog.Error("Configuration Update Command is nil")
		return
	}

	command := evt.NasMsg.ConfigurationUpdateCommand
	ackRequested := configurationUpdateNeedsAck(command)
	registrationRequested := command.ConfigurationUpdateIndication != nil &&
		command.ConfigurationUpdateIndication.GetRED() == 1
	logger.NWuCPLog.Infof(
		"Handled Configuration Update Command: ACK requested=%t RED=%t, NITZ full-name=%t short-name=%t local-time-zone=%t universal-time=%t daylight-saving=%t",
		ackRequested,
		registrationRequested,
		command.FullNameForNetwork != nil,
		command.ShortNameForNetwork != nil,
		command.LocalTimeZone != nil,
		command.UniversalTimeAndLocalTimeZone != nil,
		command.NetworkDaylightSavingTime != nil,
	)

	if !ackRequested {
		return
	}

	SendNasMsg(s.Context().RanUeContext, s.Context().N3IWFRanUe.TCPConnection,
		nasPacket.GetConfigurationUpdateComplete())
	logger.NWuCPLog.Info("Sent Configuration Update Complete")
}

func (s *Server) handleStartNwucpConnEvt() {
	errChan := make(chan error)

	s.serverWg.Add(1)
	go s.serveConn(errChan)
	if err, ok := <-errChan; ok {
		logger.NWuCPLog.Errorf("NWUCP service startup failed: %+v", err)
		return
	}
}

func (s *Server) handleRegistrationAccept(evt *context.HandleRegistrationAcceptEvt) {
	nwucpLog := logger.NWuCPLog
	nwucpLog.Tracef("Get Registration Accept")

	n3ueSelf := s.Context()
	nasMsg := evt.NasMsg
	n3ueSelf.RanUeContext.DLCount.AddOne()
	n3ueSelf.GUTI = nasMsg.RegistrationAccept.GUTI5G

	// send NAS Registration Complete Msg
	pdu := nasPacket.GetRegistrationComplete(nil)
	SendNasMsg(n3ueSelf.RanUeContext, n3ueSelf.N3IWFRanUe.TCPConnection, pdu)

	s.SendProcedureEvt(context.NewSuccessRegistrationEvt())
}

func (s *Server) handleDLNASTransport(evt *context.HandleDLNASTransportEvt) {
	nwucpLog := logger.NWuCPLog
	nwucpLog.Tracef("Get DLNAS Transport")

	n3ueSelf := s.Context()
	nasMsg := evt.NasMsg

	payloadContainer := nasMsg.DLNASTransport.PayloadContainer
	byteArray := payloadContainer.Buffer[:payloadContainer.Len]
	if err := nasMsg.GsmMessageDecode(&byteArray); err != nil {
		nwucpLog.Errorf("NAS Decode Fail: %+v", err)
		return
	}

	switch nasMsg.GsmMessage.GetMessageType() {
	case nas.MsgTypePDUSessionEstablishmentAccept:
		nwucpLog.Tracef("Get PDUSession Establishment Accept")

		pduAddress, err := nasPacket.GetPDUAddress(nasMsg.PDUSessionEstablishmentAccept)
		if err != nil {
			nwucpLog.Errorf("GetPDUAddress Fail: %+v", err)
			return
		}

		pduSessionID := nasMsg.PDUSessionEstablishmentAccept.GetPDUSessionID()
		session := n3ueSelf.GetOrCreatePDUSession(pduSessionID)
		xfrmiName, upIPAddr, qosInfo, err := session.WaitUserPlaneReady()
		if err != nil {
			nwucpLog.Errorf("User plane for accepted PDU session %d is not ready: %v", pduSessionID, err)
			return
		}
		session.PDUAddress = append(net.IP(nil), pduAddress...)
		nwucpLog.Infof("PDU session %d address: %s", pduSessionID, pduAddress.String())

		newGREName := fmt.Sprintf("%s-pdu-%d", n3ueSelf.N3ueInfo.GreIfaceName, pduSessionID)

		// Create VRF
		vrfName := fmt.Sprintf("vrf-pdu-%d", pduSessionID)
		vrfTableID := 100 + int(pduSessionID)

		vrfLink, err := vrf.CreateOrGetVRF(vrfName, vrfTableID)
		if err != nil {
			nwucpLog.Errorf("Create VRF failed:%s: %v", vrfName, err)
			return
		}
		nwucpLog.Infof("Use VRF %s (Table %d)", vrfName, vrfTableID)

		var linkGREs map[uint8]*netlink.Link
		if linkGREs, err = gre.SetupGreTunnels(newGREName, xfrmiName, n3ueSelf.UEInnerAddr.IP,
			upIPAddr, pduAddress, qosInfo); err != nil {
			nwucpLog.Errorf("Setup GRE tunnel %s Fail: %+v", newGREName, err)
			return
		}

		qfiToTargetMap, err := nasPacket.GetQFItoTargetMap(nasMsg.PDUSessionEstablishmentAccept)
		if err != nil {
			nwucpLog.Errorf("GetQFItoTargetMap Fail: %+v", err)
			return
		}

		// Add route
		for qfi, link := range linkGREs {
			tunnel := *link
			priority := 1 // lower is higher (1 ~ 7)

			var remoteAddress nasType.PacketFilterIPv4RemoteAddress
			var ok bool
			if qfi == uint8(1) { // default qfi
				remoteAddress.Address = net.IPv4zero
				remoteAddress.Mask = net.IPv4Mask(0, 0, 0, 0)
				priority = 7
			} else if remoteAddress, ok = qfiToTargetMap[qfi]; !ok {
				nwucpLog.Errorf("not found target address for QFI [%v] from NAS", qfi)
				continue
			}
			// Bind GRE interface to VRF
			if err := netlink.LinkSetMaster(tunnel, vrfLink); err != nil {
				nwucpLog.Errorf("Failed to bind %s to VRF %s", tunnel.Attrs().Name, vrfName)
				continue
			}
			nwucpLog.Infof("Interface %s bound to VRF %s", tunnel.Attrs().Name, vrfName)

			nwucpLog.Infof("Add route: QFI[%+v] remote address[%+v]", qfi, remoteAddress)
			upRoute := &netlink.Route{
				LinkIndex: tunnel.Attrs().Index,
				Dst: &net.IPNet{
					IP:   remoteAddress.Address,
					Mask: remoteAddress.Mask,
				},
				Priority: priority,
				Table:    vrfTableID,
			}
			if err := netlink.RouteAdd(upRoute); err != nil {
				nwucpLog.Warnf("netlink.RouteAdd: %+v", err)
			}
		}

		session.GRETunnels = linkGREs
		session.VRF, session.VRFName, session.RouteTable = vrfLink, vrfName, vrfTableID
		s.SendProcedureEvt(context.NewPduSessionEstablishedEvt(pduSessionID))
		if len(n3ueSelf.RequestedPduSessionIDs) != 0 {
			s.SendNwucpEvt(context.NewStartPduSessionEstablishmentEvt())
		}
	case nas.MsgTypePDUSessionReleaseCommand:
		s.handlePduSessionReleaseCommand(nasMsg.PDUSessionReleaseCommand)
	}
}

func (s *Server) handlePduSessionReleaseCommand(command *nasMessage.PDUSessionReleaseCommand) {
	if command == nil {
		logger.NWuCPLog.Error("PDU session release command is nil")
		return
	}
	id := command.GetPDUSessionID()
	n3ueSelf := s.Context()
	session := n3ueSelf.GetPDUSession(id)
	if session == nil {
		logger.NWuCPLog.Warnf("Release command for unknown PDU session %d", id)
		return
	}

	sst, err := strconv.ParseInt(factory.N3ueInfo.SmPolicy[0].SNSSAI.SST, 16, 32)
	if err != nil {
		logger.NWuCPLog.Errorf("Parse SST for release complete: %v", err)
		return
	}
	snssai := &models.Snssai{Sst: int32(sst), Sd: factory.N3ueInfo.SmPolicy[0].SNSSAI.SD}
	pdu := nasPacket.GetUlNasTransport_PduSessionReleaseComplete(id,
		nasMessage.ULNASTransportRequestTypeExistingPduSession,
		factory.N3ueInfo.SmPolicy[0].DNN, snssai, command.GetPTI())
	SendNasMsg(n3ueSelf.RanUeContext, n3ueSelf.N3IWFRanUe.TCPConnection, pdu)

	greLinks, vrfLink := session.NetworkLinks()
	for _, link := range greLinks {
		if err := netlink.LinkDel(link); err != nil {
			logger.NWuCPLog.Warnf("Delete session %d GRE %s: %v", id, link.Attrs().Name, err)
		}
	}
	if vrfLink != nil {
		if err := netlink.LinkDel(vrfLink); err != nil {
			logger.NWuCPLog.Warnf("Delete session %d VRF %s: %v", id, vrfLink.Attrs().Name, err)
		}
	}
	n3ueSelf.DeletePDUSession(id)
	logger.NWuCPLog.Infof("PDU session %d released; local GRE/VRF state removed", id)
}

func (s *Server) handleReleasePduSessionEvt(id uint8) {
	if id < 1 || id > 15 || s.Context().GetPDUSession(id) == nil {
		logger.NWuCPLog.Warnf("Cannot release inactive/invalid PDU session %d", id)
		return
	}
	pdu := nasPacket.GetUlNasTransport_PduSessionReleaseRequest(id)
	SendNasMsg(s.Context().RanUeContext, s.Context().N3IWFRanUe.TCPConnection, pdu)
	logger.NWuCPLog.Infof("Requested release of PDU session %d", id)
}

func (s *Server) handleEstablishPduSessionEvt(id uint8) {
	if id < 1 || id > 15 || s.Context().GetPDUSession(id) != nil {
		logger.NWuCPLog.Warnf("Cannot establish active/invalid PDU session %d", id)
		return
	}
	s.Context().RequestedPduSessionIDs = append(s.Context().RequestedPduSessionIDs, id)
	s.handleStartPduSessionEstablishmentEvt()
}

func (s *Server) handleStartPduSessionEstablishmentEvt() {
	nwucpLog := logger.NWuCPLog
	nwucpLog.Tracef("Get Start PduSession Establishment")

	n3ueSelf := s.Context()
	if len(n3ueSelf.RequestedPduSessionIDs) == 0 {
		return
	}
	pduSessionID := n3ueSelf.RequestedPduSessionIDs[0]
	n3ueSelf.RequestedPduSessionIDs = n3ueSelf.RequestedPduSessionIDs[1:]
	if pduSessionID < 1 || pduSessionID > 15 {
		nwucpLog.Errorf("Invalid PDU session ID %d", pduSessionID)
		return
	}
	n3ueSelf.PendingPduSessionID = pduSessionID
	n3ueSelf.GetOrCreatePDUSession(pduSessionID)
	err := SendPduSessionEstablishmentRequest(n3ueSelf.RanUeContext, n3ueSelf.N3IWFRanUe.TCPConnection, pduSessionID)
	if err != nil {
		nwucpLog.Errorf("SendPduSessionEstablishmentRequest Fail: %+v", err)
		return
	}
	nwucpLog.Infof("Requested PDU session %d DNN %s S-NSSAI SST=%s SD=%s", pduSessionID,
		factory.N3ueInfo.SmPolicy[0].DNN, factory.N3ueInfo.SmPolicy[0].SNSSAI.SST,
		factory.N3ueInfo.SmPolicy[0].SNSSAI.SD)
}

func (s *Server) handleSendDeregistrationEvt() {
	nwucpLog := logger.NWuCPLog
	nwucpLog.Tracef("Get Send Deregistration")

	s.SendDeregistration()
}

func (s *Server) handleDeregistrationReqUeTerminated(evt *context.HandleDeregistrationReqUeTerminatedEvt) {
	nwucpLog := logger.NWuCPLog
	nwucpLog.Tracef("Get Deregistration Request UE Terminated")

	n3ueSelf := s.Context()
	nasMsg := evt.NasMsg
	deregistrationRequest := nasMsg.DeregistrationRequestUETerminatedDeregistration
	if deregistrationRequest == nil {
		nwucpLog.Errorf("Deregistration Request UE Terminated is nil")
		return
	}

	deregType := deregistrationRequest.SpareHalfOctetAndDeregistrationType
	deregistrationAccept := nasPacket.GetDeregistrationAccept()
	if deregType.GetReRegistrationRequired() == 1 {
		nwucpLog.Infof("handleDeregistrationReqUeTerminated(): Core network triggered re-registration required")
		n3ueSelf.ReRegistrationRequired = true
	}

	// Send Deregistration Accept
	SendNasMsg(n3ueSelf.RanUeContext, n3ueSelf.N3IWFRanUe.TCPConnection, deregistrationAccept)

	// Stop TCP connection
	s.StopTCPConnection()
}
