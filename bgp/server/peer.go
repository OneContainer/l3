// peer.go
package server

import (
	"fmt"
	"l3/bgp/config"
	"l3/bgp/packet"
	"net"
	"sync/atomic"
	"time"
	"utils/logging"
)

type Peer struct {
	Server     *BGPServer
	logger     *logging.Writer
	Global     *config.GlobalConfig
	PeerGroup  *config.PeerGroupConfig
	Neighbor   *config.Neighbor
	fsmManager *FSMManager
	BGPId      net.IP
	ASSize     uint8
	afiSafiMap map[uint32]bool
	PeerConf   config.NeighborConfig
	ifIdx      int32
	ribOut     map[string]map[uint32]*Path
}

func NewPeer(server *BGPServer, globalConf *config.GlobalConfig, peerGroup *config.PeerGroupConfig, peerConf config.NeighborConfig) *Peer {
	peer := Peer{
		Server:    server,
		logger:    server.logger,
		Global:    globalConf,
		PeerGroup: peerGroup,
		Neighbor: &config.Neighbor{
			NeighborAddress: peerConf.NeighborAddress,
			Config:          peerConf,
		},
		BGPId:      net.IP{},
		afiSafiMap: make(map[uint32]bool),
		PeerConf:   config.NeighborConfig{},
		ifIdx:      -1,
		ribOut:     make(map[string]map[uint32]*Path),
	}

	peer.SetPeerConf(peerGroup, &peer.PeerConf)
	peer.SetNeighborState(&peer.PeerConf)

	if peerConf.LocalAS == peerConf.PeerAS {
		peer.Neighbor.State.PeerType = config.PeerTypeInternal
	} else {
		peer.Neighbor.State.PeerType = config.PeerTypeExternal
	}
	if peer.PeerConf.BfdEnable {
		peer.Neighbor.State.BfdNeighborState = "up"
	} else {
		peer.Neighbor.State.BfdNeighborState = "down"
	}

	peer.afiSafiMap, _ = packet.GetProtocolFromConfig(&peer.Neighbor.AfiSafis)
	peer.fsmManager = NewFSMManager(&peer, globalConf, &peerConf)
	return &peer
}

func (p *Peer) Init() {
	if p.Neighbor.State.BfdNeighborState == "down" {
		p.logger.Info(fmt.Sprintf("Neighbor's bfd state is down for %s\n", p.Neighbor.NeighborAddress))
		return
	}
	if p.fsmManager == nil {
		p.logger.Info(fmt.Sprintf("Instantiating new FSM Manager for neighbor %s\n", p.Neighbor.NeighborAddress))
		p.fsmManager = NewFSMManager(p, &p.Server.BgpConfig.Global.Config, &p.PeerConf)
	}

	go p.fsmManager.Init()
}

func (p *Peer) Cleanup() {
	p.fsmManager.closeCh <- true
	p.fsmManager = nil
}

func (p *Peer) StopFSM(msg string) {
	p.fsmManager.stopFSMCh <- msg
}

func (p *Peer) SetNeighborState(peerConf *config.NeighborConfig) {
	p.Neighbor.State = config.NeighborState{
		PeerAS:                  peerConf.PeerAS,
		LocalAS:                 peerConf.LocalAS,
		AuthPassword:            peerConf.AuthPassword,
		Description:             peerConf.Description,
		NeighborAddress:         peerConf.NeighborAddress,
		IfIndex:                 peerConf.IfIndex,
		RouteReflectorClusterId: peerConf.RouteReflectorClusterId,
		RouteReflectorClient:    peerConf.RouteReflectorClient,
		MultiHopEnable:          peerConf.MultiHopEnable,
		MultiHopTTL:             peerConf.MultiHopTTL,
		ConnectRetryTime:        peerConf.ConnectRetryTime,
		HoldTime:                peerConf.HoldTime,
		KeepaliveTime:           peerConf.KeepaliveTime,
		PeerGroup:               peerConf.PeerGroup,
		AddPathsRx:              false,
		AddPathsMaxTx:           0,
	}
}

func (p *Peer) UpdateNeighborConf(nConf config.NeighborConfig) {
	p.Neighbor.NeighborAddress = nConf.NeighborAddress
	p.Neighbor.Config = nConf
	p.PeerConf = config.NeighborConfig{}
	if nConf.PeerGroup != p.PeerGroup.Name {
		if peerGroup, ok := p.Server.BgpConfig.PeerGroups[nConf.PeerGroup]; ok {
			p.GetNeighConfFromPeerGroup(&peerGroup.Config, &p.PeerConf)
		}
	}
	p.GetConfFromNeighbor(&p.Neighbor.Config, &p.PeerConf)
	p.SetNeighborState(&p.PeerConf)
}

func (p *Peer) UpdatePeerGroup(peerGroup *config.PeerGroupConfig) {
	p.PeerGroup = peerGroup
	p.PeerConf = config.NeighborConfig{}
	p.SetPeerConf(peerGroup, &p.PeerConf)
	p.SetNeighborState(&p.PeerConf)
}

func (p *Peer) SetPeerConf(peerGroup *config.PeerGroupConfig, peerConf *config.NeighborConfig) {
	p.GetNeighConfFromGlobal(peerConf)
	p.GetNeighConfFromPeerGroup(peerGroup, peerConf)
	p.GetConfFromNeighbor(&p.Neighbor.Config, peerConf)
}

func (p *Peer) GetNeighConfFromGlobal(peerConf *config.NeighborConfig) {
	peerConf.LocalAS = p.Server.BgpConfig.Global.Config.AS
}

func (p *Peer) GetNeighConfFromPeerGroup(groupConf *config.PeerGroupConfig, peerConf *config.NeighborConfig) {
	globalAS := peerConf.LocalAS
	if groupConf != nil {
		peerConf.BaseConfig = groupConf.BaseConfig
	}
	if peerConf.LocalAS == 0 {
		peerConf.LocalAS = globalAS
	}
}

func (p *Peer) GetConfFromNeighbor(inConf *config.NeighborConfig, outConf *config.NeighborConfig) {
	if inConf.PeerAS != 0 {
		outConf.PeerAS = inConf.PeerAS
	}

	if inConf.LocalAS != 0 {
		outConf.LocalAS = inConf.LocalAS
	}

	if inConf.AuthPassword != "" {
		outConf.AuthPassword = inConf.AuthPassword
	}

	if inConf.Description != "" {
		outConf.Description = inConf.Description
	}

	if inConf.RouteReflectorClusterId != 0 {
		outConf.RouteReflectorClusterId = inConf.RouteReflectorClusterId
	}

	if inConf.RouteReflectorClient != false {
		outConf.RouteReflectorClient = inConf.RouteReflectorClient
	}

	if inConf.MultiHopEnable != false {
		outConf.MultiHopEnable = inConf.MultiHopEnable
	}

	if inConf.MultiHopTTL != 0 {
		outConf.MultiHopTTL = inConf.MultiHopTTL
	}

	if inConf.ConnectRetryTime != 0 {
		outConf.ConnectRetryTime = inConf.ConnectRetryTime
	}

	if inConf.HoldTime != 0 {
		outConf.HoldTime = inConf.HoldTime
	}

	if inConf.KeepaliveTime != 0 {
		outConf.KeepaliveTime = inConf.KeepaliveTime
	}

	if inConf.AddPathsRx != false {
		outConf.AddPathsRx = inConf.AddPathsRx
	}

	if inConf.AddPathsMaxTx != 0 {
		outConf.AddPathsMaxTx = inConf.AddPathsMaxTx
	}

	if inConf.BfdEnable != false {
		outConf.BfdEnable = inConf.BfdEnable
	}

	outConf.NeighborAddress = inConf.NeighborAddress
	outConf.IfIndex = inConf.IfIndex
	outConf.PeerGroup = inConf.PeerGroup
}

func (p *Peer) setIfIdx(ifIdx int32) {
	p.ifIdx = ifIdx
}

func (p *Peer) getIfIdx() int32 {
	return p.ifIdx
}

func (p *Peer) AcceptConn(conn *net.TCPConn) {
	if p.Neighbor.State.BfdNeighborState == "down" {
		p.logger.Info(fmt.Sprintf("Neighbor's bfd state is down for %s\n", p.Neighbor.NeighborAddress))
		(*conn).Close()
		return
	}
	if p.fsmManager == nil {
		p.logger.Info(fmt.Sprintf("FSM Manager is not instantiated yet for neighbor %s\n", p.Neighbor.NeighborAddress))
		(*conn).Close()
		return
	}
	p.fsmManager.acceptCh <- conn
}

func (p *Peer) Command(command int) {
	if p.fsmManager == nil {
		p.logger.Info(fmt.Sprintf("FSM Manager is not instantiated yet for neighbor %s\n", p.Neighbor.NeighborAddress))
		return
	}
	p.fsmManager.commandCh <- command
}

func (p *Peer) IsInternal() bool {
	return p.PeerConf.PeerAS == p.PeerConf.LocalAS
}

func (p *Peer) IsExternal() bool {
	return p.PeerConf.LocalAS != p.PeerConf.PeerAS
}

func (p *Peer) IsRouteReflectorClient() bool {
	return p.PeerConf.RouteReflectorClient
}

func (p *Peer) SendKeepAlives(conn *net.TCPConn) {
	bgpKeepAliveMsg := packet.NewBGPKeepAliveMessage()
	var num int
	var err error

	for {
		select {
		case <-time.After(time.Second * 1):
			p.logger.Info(fmt.Sprintln("send the packet ..."))
			packet, _ := bgpKeepAliveMsg.Encode()
			num, err = conn.Write(packet)
			if err != nil {
				p.logger.Info(fmt.Sprintln("Conn.Write failed with error:", err))
			}
			p.logger.Info(fmt.Sprintln("Conn.Write succeeded. sent %d", num, "bytes"))
		}
	}
}

func (p *Peer) SetPeerAttrs(bgpId net.IP, asSize uint8, holdTime uint32, keepaliveTime uint32,
	addPathFamily map[packet.AFI]map[packet.SAFI]uint8) {
	p.BGPId = bgpId
	p.ASSize = asSize
	p.Neighbor.State.HoldTime = holdTime
	p.Neighbor.State.KeepaliveTime = keepaliveTime
	for afi, safiMap := range addPathFamily {
		if afi == packet.AfiIP {
			for _, val := range safiMap {
				if (val & packet.BGPCapAddPathRx) != 0 {
					p.logger.Info(fmt.Sprintf("SetPeerAttrs - Neighbor %s set add paths maxtx to %d\n", p.Neighbor.NeighborAddress, p.PeerConf.AddPathsMaxTx))
					p.Neighbor.State.AddPathsMaxTx = p.PeerConf.AddPathsMaxTx
				}
				if (val & packet.BGPCapAddPathTx) != 0 {
					p.logger.Info(fmt.Sprintf("SetPeerAttrs - Neighbor %s set add paths rx to %s\n", p.Neighbor.NeighborAddress, p.PeerConf.AddPathsRx))
					p.Neighbor.State.AddPathsRx = true
				}
			}
		}
	}
}

func (p *Peer) getAddPathsMaxTx() int {
	return int(p.Neighbor.State.AddPathsMaxTx)
}

func (p *Peer) updatePathAttrs(bgpMsg *packet.BGPMessage, path *Path) bool {
	if p.Neighbor.Transport.Config.LocalAddress == nil {
		p.logger.Err(fmt.Sprintf("Neighbor %s: Can't send Update message, FSM is not in Established state\n",
			p.Neighbor.NeighborAddress))
		return false
	}

	if bgpMsg == nil || bgpMsg.Body.(*packet.BGPUpdate).PathAttributes == nil {
		p.logger.Err(fmt.Sprintf("Neighbor %s: Path attrs not found in BGP Update message\n", p.Neighbor.NeighborAddress))
		return false
	}

	if len(bgpMsg.Body.(*packet.BGPUpdate).NLRI) == 0 {
		return true
	}

	if p.ASSize == 2 {
		packet.Convert4ByteTo2ByteASPath(bgpMsg)
	}

	if p.IsInternal() {
		if path.peer != nil && (path.peer.IsRouteReflectorClient() || p.IsRouteReflectorClient()) {
			packet.AddOriginatorId(bgpMsg, path.peer.BGPId)
			packet.AddClusterId(bgpMsg, path.peer.PeerConf.RouteReflectorClusterId)
		} else {
			packet.SetNextHop(bgpMsg, p.Neighbor.Transport.Config.LocalAddress)
			packet.SetLocalPref(bgpMsg, path.GetPreference())
		}
	} else {
		// Do change these path attrs for local routes
		if path.peer != nil {
			packet.RemoveMultiExitDisc(bgpMsg)
		}
		packet.PrependAS(bgpMsg, p.PeerConf.LocalAS, p.ASSize)
		packet.SetNextHop(bgpMsg, p.Neighbor.Transport.Config.LocalAddress)
		packet.RemoveLocalPref(bgpMsg)
	}

	return true
}

func (p *Peer) PeerConnEstablished(conn *net.Conn) {
	host, _, err := net.SplitHostPort((*conn).LocalAddr().String())
	if err != nil {
		p.logger.Err(fmt.Sprintf("Neighbor %s: Can't find local address from the peer connection: %s", p.Neighbor.NeighborAddress, (*conn).LocalAddr()))
		return
	}
	p.Neighbor.Transport.Config.LocalAddress = net.ParseIP(host)
	//p.Server.PeerConnEstCh <- p.Neighbor.NeighborAddress.String()
}

func (p *Peer) PeerConnBroken(fsmCleanup bool) {
	if p.Neighbor.Transport.Config.LocalAddress != nil {
		p.Neighbor.Transport.Config.LocalAddress = nil
		//p.Server.PeerConnBrokenCh <- p.Neighbor.NeighborAddress.String()
	}

	p.Neighbor.State.ConnectRetryTime = p.PeerConf.ConnectRetryTime
	p.Neighbor.State.HoldTime = p.PeerConf.HoldTime
	p.Neighbor.State.KeepaliveTime = p.PeerConf.KeepaliveTime
	p.Neighbor.State.AddPathsRx = false
	p.Neighbor.State.AddPathsMaxTx = 0

}

func (p *Peer) FSMStateChange(state BGPFSMState) {
	p.logger.Info(fmt.Sprintf("Neighbor %s: FSMStateChange %d", p.Neighbor.NeighborAddress, state))
	p.Neighbor.State.SessionState = uint32(state)
}

func (p *Peer) sendUpdateMsg(msg *packet.BGPMessage, path *Path) {
	if path != nil && path.peer != nil {
		if path.peer.IsInternal() {

			if p.IsInternal() && !path.peer.IsRouteReflectorClient() && !p.IsRouteReflectorClient() {
				return
			}
		}

		// Don't send the update to the peer that sent the update.
		if p.PeerConf.NeighborAddress.String() == path.peer.PeerConf.NeighborAddress.String() {
			return
		}
	}

	if p.updatePathAttrs(msg, path) {
		atomic.AddUint32(&p.Neighbor.State.Queues.Output, 1)
		p.fsmManager.SendUpdateMsg(msg)
	}

}

func (p *Peer) isAdvertisable(path *Path) bool {
	if path != nil && path.peer != nil {
		if path.peer.IsInternal() {

			if p.IsInternal() && !path.peer.IsRouteReflectorClient() && !p.IsRouteReflectorClient() {
				return false
			}
		}

		// Don't send the update to the peer that sent the update.
		if p.PeerConf.NeighborAddress.String() == path.peer.PeerConf.NeighborAddress.String() {
			return false
		}
	}

	return true
}

func (p *Peer) calculateAddPathsAdvertisements(dest *Destination, path *Path, newUpdated map[*Path][]packet.NLRI,
	withdrawList []packet.NLRI, addPathsTx int) (map[*Path][]packet.NLRI, []packet.NLRI) {
	pathIdMap := make(map[uint32]*Path)
	ip := dest.ipPrefix.Prefix.String()

	if _, ok := p.ribOut[ip]; !ok {
		p.logger.Info(fmt.Sprintf("Neighbor %s: calculateAddPathsAdvertisements - processing updates, dest %s not found in rib out",
			p.Neighbor.NeighborAddress, ip))
		p.ribOut[ip] = make(map[uint32]*Path)
	}

	if p.isAdvertisable(path) {
		route := dest.locRibPathRoute
		if path != nil { // Loc-RIB path changed
			if _, ok := newUpdated[path]; !ok {
				newUpdated[path] = make([]packet.NLRI, 0)
			}
			nlri := packet.NewExtNLRI(route.outPathId, *dest.ipPrefix)
			newUpdated[path] = append(newUpdated[path], nlri)
		} else {
			path = dest.locRibPath
		}
		pathIdMap[route.outPathId] = path
	}

	for i := 0; i < len(dest.addPaths) && len(pathIdMap) < (addPathsTx-1); i++ {
		route := dest.GetPathRoute(dest.addPaths[i])
		if route != nil && p.isAdvertisable(dest.addPaths[i]) {
			pathIdMap[route.outPathId] = dest.addPaths[i]
		}
	}

	ribPathMap, _ := p.ribOut[ip]
	for ribPathId, ribPath := range ribPathMap {
		if path, ok := pathIdMap[ribPathId]; !ok {
			nlri := packet.NewExtNLRI(ribPathId, *dest.ipPrefix)
			withdrawList = append(withdrawList, nlri)
			delete(p.ribOut[ip], ribPathId)
		} else if ribPath == path {
			delete(pathIdMap, ribPathId)
		} else if ribPath != path {
			if _, ok := newUpdated[path]; !ok {
				newUpdated[path] = make([]packet.NLRI, 0)
			}
			nlri := packet.NewExtNLRI(ribPathId, *dest.ipPrefix)
			newUpdated[path] = append(newUpdated[path], nlri)
			p.ribOut[ip][ribPathId] = path
			delete(pathIdMap, ribPathId)
		}
	}

	for pathId, path := range pathIdMap {
		if _, ok := newUpdated[path]; !ok {
			newUpdated[path] = make([]packet.NLRI, 0)
		}
		nlri := packet.NewExtNLRI(pathId, *dest.ipPrefix)
		newUpdated[path] = append(newUpdated[path], nlri)
		p.ribOut[ip][pathId] = path
		delete(pathIdMap, pathId)
	}

	return newUpdated, withdrawList
}

func (p *Peer) SendUpdate(updated map[*Path][]*Destination, withdrawn []*Destination, withdrawPath *Path,
	updatedAddPaths []*Destination) {
	p.logger.Info(fmt.Sprintf("Neighbor %s: Send update message valid routes:%v, withdraw routes:%v",
		p.Neighbor.NeighborAddress, updated, withdrawn))
	if p.Neighbor.Transport.Config.LocalAddress == nil {
		p.logger.Err(fmt.Sprintf("Neighbor %s: Can't send Update message, FSM is not in Established state",
			p.Neighbor.NeighborAddress))
		return
	}

	addPathsTx := p.getAddPathsMaxTx()
	withdrawList := make([]packet.NLRI, 0)
	newUpdated := make(map[*Path][]packet.NLRI)
	if len(withdrawn) > 0 {
		for _, dest := range withdrawn {
			if dest != nil {
				ip := dest.ipPrefix.Prefix.String()
				if addPathsTx > 0 {
					pathIdMap, ok := p.ribOut[ip]
					if !ok {
						p.logger.Err(fmt.Sprintf("Neighbor %s: SendUpdate - processing withdraws, dest %s not found in rib out",
							p.Neighbor.NeighborAddress, ip))
						continue
					}
					for pathId, _ := range pathIdMap {
						nlri := packet.NewExtNLRI(pathId, *dest.ipPrefix)
						withdrawList = append(withdrawList, nlri)
					}
					delete(p.ribOut, ip)
				} else {
					withdrawList = append(withdrawList, dest.ipPrefix)
					delete(p.ribOut, ip)
				}
			}
		}
	}

	for path, destinations := range updated {
		for _, dest := range destinations {
			if dest != nil {
				ip := dest.ipPrefix.Prefix.String()
				if addPathsTx > 0 {
					newUpdated, withdrawList = p.calculateAddPathsAdvertisements(dest, path, newUpdated, withdrawList,
						addPathsTx)
				} else {
					if !p.isAdvertisable(path) {
						withdrawList = append(withdrawList, dest.ipPrefix)
						delete(p.ribOut, ip)
					} else {
						route := dest.locRibPathRoute
						pathId := route.outPathId
						if _, ok := p.ribOut[ip]; !ok {
							p.ribOut[ip] = make(map[uint32]*Path)
						}
						for ribPathId, _ := range p.ribOut[ip] {
							if pathId != ribPathId {
								delete(p.ribOut[ip], ribPathId)
							}
						}
						if ribPath, ok := p.ribOut[ip][pathId]; !ok || ribPath != path {
							if _, ok := newUpdated[path]; !ok {
								newUpdated[path] = make([]packet.NLRI, 0)
							}
							newUpdated[path] = append(newUpdated[path], dest.ipPrefix)
						}
						p.ribOut[ip][pathId] = path
					}
				}
			}
		}
	}

	if addPathsTx > 0 {
		for _, dest := range updatedAddPaths {
			newUpdated, withdrawList = p.calculateAddPathsAdvertisements(dest, nil, newUpdated, withdrawList, addPathsTx)
		}
	}

	if len(withdrawList) > 0 {
		p.logger.Info(fmt.Sprintf("Neighbor %s: Send update message withdraw routes:%+v",
			p.Neighbor.NeighborAddress, withdrawList))
		updateMsg := packet.NewBGPUpdateMessage(withdrawList, nil, nil)
		p.sendUpdateMsg(updateMsg.Clone(), withdrawPath)
		withdrawList = withdrawList[:0]
	}

	for path, nlriList := range newUpdated {
		p.logger.Info(fmt.Sprintf("Neighbor %s: Send update message valid routes:%+v",
			p.Neighbor.NeighborAddress, nlriList))
		updateMsg := packet.NewBGPUpdateMessage(make([]packet.NLRI, 0), path.pathAttrs, nlriList)
		p.sendUpdateMsg(updateMsg.Clone(), path)
	}
}
