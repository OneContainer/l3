//
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//	 Unless required by applicable law or agreed to in writing, software
//	 distributed under the License is distributed on an "AS IS" BASIS,
//	 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	 See the License for the specific language governing permissions and
//	 limitations under the License.
//
//  _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  |
// |  |__   |  |     |  |__   \  V  /     |   (----` \   \/    \/   /  |  | `---|  |----`|  ,----'|  |__|  |
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   |
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  |
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__|
//

// server.go
package server

import (
	"errors"
	"fmt"
	"l3/bgp/config"
	"l3/bgp/fsm"
	"l3/bgp/packet"
	bgppolicy "l3/bgp/policy"
	bgprib "l3/bgp/rib"
	"l3/bgp/utils"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"utils/dbutils"
	"utils/eventUtils"
	"utils/logging"
	"utils/netUtils"
	"utils/patriciaDB"
	utilspolicy "utils/policy"
	"utils/policy/policyCommonDefs"
	"utils/statedbclient"
)

type GlobalUpdate struct {
	OldConfig config.GlobalConfig
	NewConfig config.GlobalConfig
	AttrSet   []bool
}

type PeerUpdate struct {
	OldPeer config.NeighborConfig
	NewPeer config.NeighborConfig
	AttrSet []bool
}

type PeerGroupUpdate struct {
	OldGroup config.PeerGroupConfig
	NewGroup config.PeerGroupConfig
	AttrSet  []bool
}

type AggUpdate struct {
	OldAgg  config.BGPAggregate
	NewAgg  config.BGPAggregate
	AttrSet []bool
}

type PolicyParams struct {
	CreateType      int
	DeleteType      int
	route           *bgprib.Route
	dest            *bgprib.Destination
	updated         *(map[uint32]map[*bgprib.Path][]*bgprib.Destination)
	withdrawn       *([]*bgprib.Destination)
	updatedAddPaths *([]*bgprib.Destination)
}

type IntfEntry struct {
	Name string
}

type BGPServer struct {
	logger           *logging.Writer
	policyManager    *bgppolicy.BGPPolicyManager
	locRibPE         map[uint32]*bgppolicy.LocRibPolicyEngine
	ribInPE          *bgppolicy.AdjRibPPolicyEngine
	ribOutPE         *bgppolicy.AdjRibPPolicyEngine
	listener         *net.TCPListener
	listenerIPv6     *net.TCPListener
	ifaceMgr         *utils.InterfaceMgr
	BgpConfig        config.Bgp
	GlobalConfigCh   chan GlobalUpdate
	AddPeerCh        chan PeerUpdate
	RemPeerCh        chan string
	AddPeerGroupCh   chan PeerGroupUpdate
	RemPeerGroupCh   chan string
	AddAggCh         chan AggUpdate
	RemAggCh         chan config.BGPAggregate
	PeerFSMConnCh    chan fsm.PeerFSMConn
	PeerConnEstCh    chan string
	PeerConnBrokenCh chan string
	PeerCommandCh    chan config.PeerCommand
	ReachabilityCh   chan config.ReachabilityInfo
	BGPPktSrcCh      chan *packet.BGPPktSrc
	BfdCh            chan config.BfdInfo
	IntfCh           chan config.IntfStateInfo
	IntfMapCh        chan config.IntfMapInfo
	RoutesCh         chan *config.RouteCh
	acceptCh         chan *net.TCPConn
	ServerUpCh       chan bool
	GlobalCfgDone    bool

	NeighborMutex   sync.RWMutex
	PeerMap         map[string]*Peer
	Neighbors       []*Peer
	LocRib          *bgprib.LocRib
	ConnRoutesPath  *bgprib.Path
	IfacePeerMap    map[int32][]string
	IntfIdNameMap   map[int32]IntfEntry
	IfNameToIfIndex map[string]int32
	ifaceIP         net.IP
	AddPathCount    int
	// all managers
	IntfMgr    config.IntfStateMgrIntf
	routeMgr   config.RouteMgrIntf
	bfdMgr     config.BfdMgrIntf
	stateDBMgr statedbclient.StateDBClient
	eventDbHdl *dbutils.DBUtil
}

func NewBGPServer(logger *logging.Writer, policyManager *bgppolicy.BGPPolicyManager, iMgr config.IntfStateMgrIntf,
	rMgr config.RouteMgrIntf, bMgr config.BfdMgrIntf, sDBMgr statedbclient.StateDBClient) *BGPServer {
	bgpServer := &BGPServer{}
	bgpServer.logger = logger
	bgpServer.policyManager = policyManager
	bgpServer.ifaceMgr = utils.NewInterfaceMgr(logger)
	bgpServer.GlobalCfgDone = false
	bgpServer.GlobalConfigCh = make(chan GlobalUpdate)
	bgpServer.AddPeerCh = make(chan PeerUpdate)
	bgpServer.RemPeerCh = make(chan string)
	bgpServer.AddPeerGroupCh = make(chan PeerGroupUpdate)
	bgpServer.RemPeerGroupCh = make(chan string)
	bgpServer.AddAggCh = make(chan AggUpdate)
	bgpServer.RemAggCh = make(chan config.BGPAggregate)
	bgpServer.PeerFSMConnCh = make(chan fsm.PeerFSMConn, 50)
	bgpServer.PeerConnEstCh = make(chan string)
	bgpServer.PeerConnBrokenCh = make(chan string)
	bgpServer.PeerCommandCh = make(chan config.PeerCommand)
	bgpServer.ReachabilityCh = make(chan config.ReachabilityInfo)
	bgpServer.BGPPktSrcCh = make(chan *packet.BGPPktSrc)
	bgpServer.BfdCh = make(chan config.BfdInfo)
	bgpServer.IntfCh = make(chan config.IntfStateInfo)
	bgpServer.IntfMapCh = make(chan config.IntfMapInfo)
	bgpServer.RoutesCh = make(chan *config.RouteCh)
	bgpServer.ServerUpCh = make(chan bool)

	bgpServer.NeighborMutex = sync.RWMutex{}
	bgpServer.PeerMap = make(map[string]*Peer)
	bgpServer.Neighbors = make([]*Peer, 0)
	bgpServer.IntfMgr = iMgr
	bgpServer.routeMgr = rMgr
	bgpServer.bfdMgr = bMgr
	bgpServer.stateDBMgr = sDBMgr
	bgpServer.LocRib = bgprib.NewLocRib(logger, rMgr, sDBMgr, &bgpServer.BgpConfig.Global.Config)
	bgpServer.IfNameToIfIndex = make(map[string]int32)
	bgpServer.IntfIdNameMap = make(map[int32]IntfEntry)
	bgpServer.IfacePeerMap = make(map[int32][]string)
	bgpServer.ifaceIP = nil
	bgpServer.AddPathCount = 0
	bgpServer.initGlobalConfig()
	bgpServer.initPolicyEngines()
	return bgpServer
}

func (s *BGPServer) initGlobalConfig() {
	s.BgpConfig = config.Bgp{}
	s.BgpConfig.Afs = make(map[uint32]*config.AddressFamily)
	for _, pfNumber := range packet.ProtocolFamilyMap {
		s.BgpConfig.Afs[pfNumber] = &config.AddressFamily{}
	}
}

func (s *BGPServer) initPolicyEngines() {
	type TraverseFuncMap struct {
		ApplyFunc   utilspolicy.EntityTraverseAndApplyPolicyfunc
		ReverseFunc utilspolicy.EntityTraverseAndReversePolicyfunc
	}
	var traverseFuncMap = map[packet.AFI]TraverseFuncMap{
		packet.AfiIP:  TraverseFuncMap{s.TraverseAndApplyBGPRib, s.TrAndRevAggForIPv4},
		packet.AfiIP6: TraverseFuncMap{s.TraverseAndApplyBGPRib, s.TrAndRevAggForIPv6},
	}
	var actionFunc bgppolicy.PolicyActionFunc
	actionFuncMap := make(map[int]bgppolicy.PolicyActionFunc)
	s.locRibPE = make(map[uint32]*bgppolicy.LocRibPolicyEngine)

	actionFunc.ApplyFunc = s.ApplyAggregateAction
	actionFunc.UndoFunc = s.UndoAggregateAction
	actionFuncMap[policyCommonDefs.PolicyActionTypeAggregate] = actionFunc
	s.logger.Infof("BGPServer: actionfuncmap=%v", actionFuncMap)

	for _, pfNumber := range packet.ProtocolFamilyMap {
		locRibPE := bgppolicy.NewLocRibPolicyEngine(s.logger)
		locRibPE.SetEntityUpdateFunc(s.UpdateRouteAndPolicyDB)
		locRibPE.SetIsEntityPresentFunc(s.DoesRouteExist)
		locRibPE.SetActionFuncs(actionFuncMap)
		afi, _ := packet.GetAfiSafi(pfNumber)
		traverseFuncs, ok := traverseFuncMap[afi]
		if !ok {
			s.logger.Crit("BGPServer: traverse funcs not found for address family", afi)
		}
		locRibPE.SetTraverseFuncs(traverseFuncs.ApplyFunc, traverseFuncs.ReverseFunc)
		s.locRibPE[pfNumber] = locRibPE
		//s.policyManager.AddPolicyEngine(locRibPE)
	}

	s.ribInPE = bgppolicy.NewAdjRibPolicyEngine(s.logger)
	s.ribOutPE = bgppolicy.NewAdjRibPolicyEngine(s.logger)

	actionFunc.ApplyFunc = s.ApplyAdjRIBAction
	actionFunc.UndoFunc = s.UndoAdjRIBAction

	s.ribInPE.SetEntityUpdateFunc(s.UpdateAdjRIBInRouteAndPolicyDB)
	s.ribInPE.SetIsEntityPresentFunc(s.DoesAdjRIBInRouteExist)
	actionFuncMap = make(map[int]bgppolicy.PolicyActionFunc)
	actionFuncMap[policyCommonDefs.PolicyActionTypeRIBIn] = actionFunc
	s.ribInPE.SetActionFuncs(actionFuncMap)
	s.ribInPE.SetTraverseFuncs(s.TraverseAndApplyAdjRibIn, s.TraverseAndReverseAdjRIBIn)
	s.policyManager.AddPolicyEngine(s.ribInPE)

	s.ribOutPE.SetEntityUpdateFunc(s.UpdateAdjRIBOutRouteAndPolicyDB)
	s.ribOutPE.SetIsEntityPresentFunc(s.DoesAdjRIBOutRouteExist)
	actionFuncMap = make(map[int]bgppolicy.PolicyActionFunc)
	actionFuncMap[policyCommonDefs.PolicyActionTypeRIBOut] = actionFunc
	s.ribOutPE.SetActionFuncs(actionFuncMap)
	s.ribOutPE.SetTraverseFuncs(s.TraverseAndApplyAdjRibOut, s.TraverseAndReverseAdjRIBOut)
	s.policyManager.AddPolicyEngine(s.ribOutPE)
}

func (s *BGPServer) createListener(proto string) (*net.TCPListener, error) {
	addr := ":" + config.BGPPort
	s.logger.Infof("Listening for incomig connections on %s", addr)
	tcpAddr, err := net.ResolveTCPAddr(proto, addr)
	if err != nil {
		s.logger.Info("ResolveTCPAddr failed with", err)
		return nil, err
	}

	listener, err := net.ListenTCP(proto, tcpAddr)
	if err != nil {
		s.logger.Info("ListenTCP failed with", err)
		return nil, err
	}

	return listener, nil
}

func (s *BGPServer) listenForPeers(listener *net.TCPListener, acceptCh chan *net.TCPConn) {
	for {
		s.logger.Info("Waiting for peer connections...")
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			s.logger.Info("AcceptTCP failed with", err)
			continue
		}
		s.logger.Info("Got a peer connection from %s", tcpConn.RemoteAddr())
		s.acceptCh <- tcpConn
	}
}

func (s *BGPServer) IsPeerLocal(peerIp string) bool {
	return s.PeerMap[peerIp].NeighborConf.RunningConf.PeerAS == s.BgpConfig.Global.Config.AS
}

func (s *BGPServer) SendUpdate(updated map[uint32]map[*bgprib.Path][]*bgprib.Destination, withdrawn,
	updatedAddPaths []*bgprib.Destination) {
	for _, peer := range s.PeerMap {
		peer.SendUpdate(updated, withdrawn, updatedAddPaths)
	}
}

func (s *BGPServer) DoesRouteExist(params interface{}) bool {
	policyParams := params.(PolicyParams)
	dest := policyParams.dest
	if dest == nil {
		s.logger.Info("BGPServer:DoesRouteExist - dest not found for ip",
			policyParams.route.Dest.BGPRouteState.Network, "prefix length",
			policyParams.route.Dest.BGPRouteState.CIDRLen)
		return false
	}

	locRibRoute := dest.GetLocRibPathRoute()
	if policyParams.route == locRibRoute {
		return true
	}

	return false
}

func (s *BGPServer) getAggPrefix(conditionsList []interface{}) *packet.IPPrefix {
	s.logger.Info("BGPServer:getAggPrefix")
	var ipPrefix *packet.IPPrefix
	var err error
	for _, condition := range conditionsList {
		switch condition.(type) {
		case utilspolicy.MatchPrefixConditionInfo:
			s.logger.Info("BGPServer:getAggPrefix - PolicyConditionTypeDstIpPrefixMatch case")
			matchPrefix := condition.(utilspolicy.MatchPrefixConditionInfo)
			s.logger.Info("BGPServer:getAggPrefix - exact prefix match conditiontype")
			ipPrefix, err = packet.ConstructIPPrefixFromCIDR(matchPrefix.Prefix.IpPrefix)
			if err != nil {
				s.logger.Info("BGPServer:getAggPrefix - ipPrefix invalid ")
				return nil
			}
			break
		default:
			s.logger.Info("BGPServer:getAggPrefix - Not a known condition type")
			break
		}
	}
	return ipPrefix
}

func (s *BGPServer) setUpdatedAddPaths(policyParams *PolicyParams,
	updatedAddPaths []*bgprib.Destination) {
	if len(updatedAddPaths) > 0 {
		addPathsMap := make(map[*bgprib.Destination]bool)
		for _, dest := range *(policyParams.updatedAddPaths) {
			addPathsMap[dest] = true
		}

		for _, dest := range updatedAddPaths {
			if !addPathsMap[dest] {
				(*policyParams.updatedAddPaths) =
					append((*policyParams.updatedAddPaths), dest)
			}
		}
	}
}

func (s *BGPServer) setWithdrawnWithAggPaths(policyParams *PolicyParams, withdrawn []*bgprib.Destination,
	sendSummaryOnly bool, updatedAddPaths []*bgprib.Destination) {
	destMap := make(map[*bgprib.Destination]bool)
	for _, dest := range *policyParams.withdrawn {
		destMap[dest] = true
	}

	aggDestMap := make(map[*bgprib.Destination]bool)
	for _, aggDestination := range withdrawn {
		aggDestMap[aggDestination] = true
		if !destMap[aggDestination] {
			s.logger.Infof("setWithdrawnWithAggPaths: add agg dest %+v to withdrawn",
				aggDestination.NLRI.GetPrefix())
			(*policyParams.withdrawn) = append((*policyParams.withdrawn), aggDestination)
		}
	}

	// There will be only one destination per aggregated path.
	// So, break out of the loop as soon as we find it.
	for protoFamily, pathDestMap := range *policyParams.updated {
		for path, destinations := range pathDestMap {
			for idx, dest := range destinations {
				if aggDestMap[dest] {
					(*policyParams.updated)[protoFamily][path][idx] = nil
					s.logger.Infof("setWithdrawnWithAggPaths: remove dest %+v from withdrawn",
						dest.NLRI.GetPrefix())
				}
			}
		}
	}

	if sendSummaryOnly {
		if policyParams.DeleteType == utilspolicy.Valid {
			for idx, dest := range *policyParams.withdrawn {
				if dest == policyParams.dest {
					s.logger.Infof("setWithdrawnWithAggPaths: remove dest %+v from withdrawn",
						dest.NLRI.GetPrefix())
					(*policyParams.withdrawn)[idx] = nil
				}
			}
		} else if policyParams.CreateType == utilspolicy.Invalid {
			if policyParams.dest != nil && policyParams.dest.LocRibPath != nil {
				found := false
				protoFamily := policyParams.dest.GetProtocolFamily()
				if destinations, ok :=
					(*policyParams.updated)[protoFamily][policyParams.dest.LocRibPath]; ok {
					for _, dest := range destinations {
						if dest == policyParams.dest {
							found = true
						}
					}
				} else {
					(*policyParams.updated)[protoFamily][policyParams.dest.LocRibPath] = make([]*bgprib.Destination, 0)
				}
				if !found {
					s.logger.Infof("setWithdrawnWithAggPaths: add dest %+v to update",
						policyParams.dest.NLRI.GetPrefix())
					(*policyParams.updated)[protoFamily][policyParams.dest.LocRibPath] = append(
						(*policyParams.updated)[protoFamily][policyParams.dest.LocRibPath], policyParams.dest)
				}
			}
		}
	}

	s.setUpdatedAddPaths(policyParams, updatedAddPaths)
}

func (s *BGPServer) setUpdatedWithAggPaths(policyParams *PolicyParams,
	updated map[uint32]map[*bgprib.Path][]*bgprib.Destination, sendSummaryOnly bool, ipPrefix *packet.IPPrefix,
	protoFamily uint32, updatedAddPaths []*bgprib.Destination) {
	var routeDest *bgprib.Destination
	var ok bool
	if routeDest, ok = s.LocRib.GetDest(ipPrefix, protoFamily, false); !ok {
		s.logger.Err("setUpdatedWithAggPaths: Did not find destination for ip", ipPrefix)
		if policyParams.dest != nil {
			routeDest = policyParams.dest
		} else {
			sendSummaryOnly = false
		}
	}

	withdrawMap := make(map[*bgprib.Destination]bool, len(*policyParams.withdrawn))
	if sendSummaryOnly {
		for _, dest := range *policyParams.withdrawn {
			withdrawMap[dest] = true
		}
	}

	for aggFamily, aggPathDestMap := range updated {
		for aggPath, aggDestinations := range aggPathDestMap {
			destMap := make(map[*bgprib.Destination]bool)
			ppUpdated := *policyParams.updated
			if _, ok := ppUpdated[aggFamily]; !ok {
				ppUpdated[aggFamily] = make(map[*bgprib.Path][]*bgprib.Destination)
			}
			if _, ok := ppUpdated[aggFamily][aggPath]; !ok {
				ppUpdated[aggFamily][aggPath] = make([]*bgprib.Destination, 0)
			} else {
				for _, dest := range ppUpdated[aggFamily][aggPath] {
					destMap[dest] = true
				}
			}

			for _, dest := range aggDestinations {
				if !destMap[dest] {
					s.logger.Infof("setUpdatedWithAggPaths: add agg dest %+v to updated", dest.NLRI.GetPrefix())
					ppUpdated[aggFamily][aggPath] = append(ppUpdated[aggFamily][aggPath], dest)
				}
			}

			if sendSummaryOnly {
				if policyParams.CreateType == utilspolicy.Valid {
					if pathDestMap, ok := ppUpdated[protoFamily]; ok {
						for path, destinations := range pathDestMap {
							for idx, dest := range destinations {
								if routeDest == dest {
									ppUpdated[protoFamily][path][idx] = nil
									s.logger.Infof("setUpdatedWithAggPaths: summaryOnly, remove dest %+v"+
										" from updated", dest.NLRI.GetPrefix())
								}
							}
						}
					}
				} else if policyParams.DeleteType == utilspolicy.Invalid {
					if !withdrawMap[routeDest] {
						s.logger.Infof("setUpdatedWithAggPaths: summaryOnly, add dest %+v to withdrawn",
							routeDest.NLRI.GetPrefix())
						(*policyParams.withdrawn) = append((*policyParams.withdrawn), routeDest)
					}
				}
			}
		}
	}

	s.setUpdatedAddPaths(policyParams, updatedAddPaths)
}

func (s *BGPServer) UndoAggregateAction(actionInfo interface{},
	conditionList []interface{}, params interface{}, policyStmt utilspolicy.PolicyStmt) {
	policyParams := params.(PolicyParams)
	ipPrefix := packet.NewIPPrefix(net.ParseIP(policyParams.route.Dest.BGPRouteState.Network),
		uint8(policyParams.route.Dest.BGPRouteState.CIDRLen))
	protoFamily := policyParams.route.Dest.GetProtocolFamily()
	aggPrefix := s.getAggPrefix(conditionList)
	aggActions := actionInfo.(utilspolicy.PolicyAggregateActionInfo)
	bgpAgg := config.BGPAggregate{
		GenerateASSet:   aggActions.GenerateASSet,
		SendSummaryOnly: aggActions.SendSummaryOnly,
	}

	s.logger.Infof("UndoAggregateAction: ipPrefix=%+v, aggPrefix=%+v", ipPrefix.Prefix, aggPrefix.Prefix)
	var updated map[uint32]map[*bgprib.Path][]*bgprib.Destination
	var withdrawn []*bgprib.Destination
	var updatedAddPaths []*bgprib.Destination
	var origDest *bgprib.Destination
	if policyParams.dest != nil {
		origDest = policyParams.dest
	}
	updated, withdrawn, updatedAddPaths = s.LocRib.RemoveRouteFromAggregate(ipPrefix, aggPrefix,
		s.BgpConfig.Global.Config.RouterId.String(), protoFamily, &bgpAgg, origDest, s.AddPathCount)

	s.logger.Infof("UndoAggregateAction: aggregate result update=%+v, withdrawn=%+v", updated, withdrawn)
	s.setWithdrawnWithAggPaths(&policyParams, withdrawn, aggActions.SendSummaryOnly, updatedAddPaths)
	s.logger.Infof("UndoAggregateAction: after updating withdraw agg paths, update=%+v, withdrawn=%+v,"+
		"policyparams.update=%+v, policyparams.withdrawn=%+v", updated, withdrawn, *policyParams.updated,
		*policyParams.withdrawn)
	return
}

func (s *BGPServer) ApplyAggregateAction(actionInfo interface{}, conditionInfo []interface{}, params interface{}) {
	policyParams := params.(PolicyParams)
	ipPrefix := packet.NewIPPrefix(net.ParseIP(policyParams.route.Dest.BGPRouteState.Network),
		uint8(policyParams.route.Dest.BGPRouteState.CIDRLen))
	protoFamily := policyParams.route.Dest.GetProtocolFamily()
	aggPrefix := s.getAggPrefix(conditionInfo)
	aggActions := actionInfo.(utilspolicy.PolicyAggregateActionInfo)
	bgpAgg := config.BGPAggregate{
		GenerateASSet:   aggActions.GenerateASSet,
		SendSummaryOnly: aggActions.SendSummaryOnly,
	}

	s.logger.Infof("ApplyAggregateAction: ipPrefix=%+v, aggPrefix=%+v", ipPrefix.Prefix, aggPrefix.Prefix)
	var updated map[uint32]map[*bgprib.Path][]*bgprib.Destination
	var withdrawn []*bgprib.Destination
	var updatedAddPaths []*bgprib.Destination
	if (policyParams.CreateType == utilspolicy.Valid) ||
		(policyParams.DeleteType == utilspolicy.Invalid) {
		s.logger.Infof("ApplyAggregateAction: CreateType= Valid or DeleteType = Invalid")
		updated, withdrawn, updatedAddPaths = s.LocRib.AddRouteToAggregate(ipPrefix, aggPrefix,
			s.BgpConfig.Global.Config.RouterId.String(), protoFamily, s.ifaceIP, &bgpAgg, s.AddPathCount)
	} else if policyParams.DeleteType == utilspolicy.Valid {
		s.logger.Infof("ApplyAggregateAction: DeleteType = Valid")
		origDest := policyParams.dest
		updated, withdrawn, updatedAddPaths = s.LocRib.RemoveRouteFromAggregate(ipPrefix, aggPrefix,
			s.BgpConfig.Global.Config.RouterId.String(), protoFamily, &bgpAgg, origDest, s.AddPathCount)
	}

	s.logger.Infof("ApplyAggregateAction: aggregate result update=%+v, withdrawn=%+v", updated, withdrawn)
	s.setUpdatedWithAggPaths(&policyParams, updated, aggActions.SendSummaryOnly, ipPrefix, protoFamily,
		updatedAddPaths)
	s.logger.Infof("ApplyAggregateAction: after updating agg paths, update=%+v, withdrawn=%+v, "+
		"policyparams.update=%+v, policyparams.withdrawn=%+v", updated, withdrawn, *policyParams.updated,
		*policyParams.withdrawn)
	return
}

func (s *BGPServer) CheckForAggregation(updated map[uint32]map[*bgprib.Path][]*bgprib.Destination, withdrawn,
	updatedAddPaths []*bgprib.Destination) (map[uint32]map[*bgprib.Path][]*bgprib.Destination, []*bgprib.Destination,
	[]*bgprib.Destination) {
	s.logger.Infof("BGPServer:checkForAggregate - start, updated %v withdrawn %v", updated, withdrawn)

	for _, dest := range withdrawn {
		if dest == nil || dest.LocRibPath == nil || dest.LocRibPath.IsAggregate() {
			continue
		}

		protoFamily := dest.GetProtocolFamily()
		if len(s.BgpConfig.Afs[protoFamily].BgpAggs) == 0 {
			s.logger.Crit("BGPServer:checkForAggregate - withdrawn, aggs for family", protoFamily, "is not set")
			continue
		}

		pe, ok := s.locRibPE[protoFamily]
		if !ok {
			s.logger.Err("BGPServer:checkForAggregate - Agg policy engine not found for family", protoFamily)
			continue
		}

		route := dest.GetLocRibPathRoute()
		if route == nil {
			s.logger.Infof("BGPServer:checkForAggregate - route not found withdraw dest %s",
				dest.NLRI.GetPrefix().String())
			continue
		}
		peEntity := utilspolicy.PolicyEngineFilterEntityParams{
			DestNetIp:  route.Dest.BGPRouteState.Network + "/" + strconv.Itoa(int(route.Dest.BGPRouteState.CIDRLen)),
			NextHopIp:  route.PathInfo.NextHop,
			DeletePath: true,
		}
		s.logger.Infof("BGPServer:checkForAggregate - withdraw dest %s policylist %v hit %v before ",
			"applying delete policy", dest.NLRI.GetPrefix().String(), route.PolicyList, route.PolicyHitCounter)
		callbackInfo := PolicyParams{
			CreateType:      utilspolicy.Invalid,
			DeleteType:      utilspolicy.Valid,
			route:           route,
			dest:            dest,
			updated:         &updated,
			withdrawn:       &withdrawn,
			updatedAddPaths: &updatedAddPaths,
		}
		pe.PolicyEngine.PolicyEngineFilter(peEntity, policyCommonDefs.PolicyPath_Export, callbackInfo)
	}

	for protoFamily, pathDestMap := range updated {
		if len(s.BgpConfig.Afs[protoFamily].BgpAggs) == 0 {
			s.logger.Crit("BGPServer:checkForAggregate - updated, aggs for family", protoFamily, "is not set")
			continue
		}

		pe, ok := s.locRibPE[protoFamily]
		if !ok {
			s.logger.Err("BGPServer:checkForAggregate - Agg policy engine not found for family", protoFamily)
			continue
		}

		for _, destinations := range pathDestMap {
			s.logger.Infof("BGPServer:checkForAggregate - update destinations %+v", destinations)
			for _, dest := range destinations {
				s.logger.Infof("BGPServer:checkForAggregate - update dest %+v", dest.NLRI.GetPrefix())
				if dest == nil || dest.LocRibPath == nil || dest.LocRibPath.IsAggregate() {
					continue
				}
				route := dest.GetLocRibPathRoute()
				s.logger.Infof("BGPServer:checkForAggregate - update dest %s policylist %v hit %v before "+
					"applying create policy", dest.NLRI.GetPrefix().String(), route.PolicyList, route.PolicyHitCounter)
				if route != nil {
					peEntity := utilspolicy.PolicyEngineFilterEntityParams{
						DestNetIp:  route.Dest.BGPRouteState.Network + "/" + strconv.Itoa(int(route.Dest.BGPRouteState.CIDRLen)),
						NextHopIp:  route.PathInfo.NextHop,
						CreatePath: true,
					}
					callbackInfo := PolicyParams{
						CreateType:      utilspolicy.Valid,
						DeleteType:      utilspolicy.Invalid,
						route:           route,
						dest:            dest,
						updated:         &updated,
						withdrawn:       &withdrawn,
						updatedAddPaths: &updatedAddPaths,
					}
					pe.PolicyEngine.PolicyEngineFilter(peEntity, policyCommonDefs.PolicyPath_Export, callbackInfo)
					s.logger.Infof("BGPServer:checkForAggregate - update dest %s policylist %v hit %v "+
						"after applying create policy", dest.NLRI.GetPrefix().String(), route.PolicyList,
						route.PolicyHitCounter)
				}
			}
		}
	}

	s.logger.Infof("BGPServer:checkForAggregate - complete, updated %v withdrawn %v", updated, withdrawn)
	return updated, withdrawn, updatedAddPaths
}

func (s *BGPServer) UpdateRouteAndPolicyDB(policyDetails utilspolicy.PolicyDetails, params interface{}) {
	var op int
	policyParams := params.(PolicyParams)
	dest := policyParams.dest
	protoFamily := dest.GetProtocolFamily()
	pe, ok := s.locRibPE[protoFamily]
	if !ok {
		s.logger.Err("UpdateRouteAndPolicyDB - Agg policy engine not found for family", protoFamily)
		return
	}

	if policyParams.DeleteType != bgppolicy.Invalid {
		op = bgppolicy.Del
	} else {
		if policyDetails.EntityDeleted == false {
			s.logger.Info("Reject action was not applied, so add this policy to the route")
			op = bgppolicy.Add
			bgppolicy.UpdateRoutePolicyState(policyParams.route, op, policyDetails.Policy, policyDetails.PolicyStmt)
		}
		policyParams.route.PolicyHitCounter++
	}
	pe.UpdatePolicyRouteMap(policyParams.route, policyDetails.Policy, op)
}

func (s *BGPServer) TraverseAndApplyBGPRib(data interface{}, updateFunc utilspolicy.PolicyApplyfunc) {
	s.logger.Infof("BGPServer:TraverseAndApplyBGPRib - start")
	policy := data.(utilspolicy.ApplyPolicyInfo)
	updated := make(map[uint32]map[*bgprib.Path][]*bgprib.Destination, 10)
	withdrawn := make([]*bgprib.Destination, 0, 10)
	updatedAddPaths := make([]*bgprib.Destination, 0)
	locRib := s.LocRib.GetLocRib()
	for protoFamily, pathDestMap := range locRib {
		if _, ok := s.locRibPE[protoFamily]; !ok {
			s.logger.Err("TraverseAndApplyBGPRib - Agg policy engine not found for family", protoFamily)
			continue
		}
		for path, destinations := range pathDestMap {
			for _, dest := range destinations {
				if !path.IsAggregatePath() {
					route := dest.GetLocRibPathRoute()
					if route == nil {
						continue
					}
					peEntity := utilspolicy.PolicyEngineFilterEntityParams{
						DestNetIp: route.Dest.BGPRouteState.Network + "/" +
							strconv.Itoa(int(route.Dest.BGPRouteState.CIDRLen)),
						NextHopIp:  route.PathInfo.NextHop,
						PolicyList: route.PolicyList,
					}
					callbackInfo := PolicyParams{
						CreateType:      utilspolicy.Invalid,
						DeleteType:      utilspolicy.Invalid,
						route:           route,
						dest:            dest,
						updated:         &updated,
						withdrawn:       &withdrawn,
						updatedAddPaths: &updatedAddPaths,
					}

					updateFunc(peEntity, policy, callbackInfo)
				}
			}
		}
	}
	s.logger.Infof("BGPServer:TraverseAndApplyBGPRib - updated %v withdrawn %v", updated, withdrawn)
	s.SendUpdate(updated, withdrawn, updatedAddPaths)
}

func (s *BGPServer) TrAndRevAggForIPv4(policyData interface{}) {
	protoFamily := packet.GetProtocolFamily(packet.AfiIP, packet.SafiUnicast)
	pe, ok := s.locRibPE[protoFamily]
	if !ok {
		s.logger.Err("TrAndRevAggForIPv4 - Agg policy engine not found for family", protoFamily)
		return
	}
	s.TraverseAndReverseBGPRib(policyData, pe)
}

func (s *BGPServer) TrAndRevAggForIPv6(policyData interface{}) {
	protoFamily := packet.GetProtocolFamily(packet.AfiIP6, packet.SafiUnicast)
	pe, ok := s.locRibPE[protoFamily]
	if !ok {
		s.logger.Err("TrAndRevAggForIPv6 - Agg policy engine not found for family", protoFamily)
		return
	}
	s.TraverseAndReverseBGPRib(policyData, pe)
}

func (s *BGPServer) TraverseAndReverseBGPRib(policyData interface{}, pe *bgppolicy.LocRibPolicyEngine) {
	policy := policyData.(utilspolicy.Policy)
	s.logger.Info("BGPServer:TraverseAndReverseBGPRib - policy", policy.Name)
	policyExtensions := policy.Extensions.(bgppolicy.PolicyExtensions)
	if len(policyExtensions.RouteList) == 0 {
		fmt.Println("No route affected by this policy, so nothing to do")
		return
	}

	updated := make(map[uint32]map[*bgprib.Path][]*bgprib.Destination, 10)
	withdrawn := make([]*bgprib.Destination, 0, 10)
	updatedAddPaths := make([]*bgprib.Destination, 0)
	var route *bgprib.Route
	for idx := 0; idx < len(policyExtensions.RouteInfoList); idx++ {
		route = policyExtensions.RouteInfoList[idx]
		dest := s.LocRib.GetDestFromIPAndLen(route.Dest.GetProtocolFamily(), route.Dest.BGPRouteState.Network,
			uint32(route.Dest.BGPRouteState.CIDRLen))

		callbackInfo := PolicyParams{
			CreateType:      utilspolicy.Invalid,
			DeleteType:      utilspolicy.Invalid,
			route:           route,
			dest:            dest,
			updated:         &updated,
			withdrawn:       &withdrawn,
			updatedAddPaths: &updatedAddPaths,
		}
		peEntity := utilspolicy.PolicyEngineFilterEntityParams{
			DestNetIp: route.Dest.BGPRouteState.Network + "/" + strconv.Itoa(int(route.Dest.BGPRouteState.CIDRLen)),
			NextHopIp: route.PathInfo.NextHop,
		}

		ipPrefix, err := bgppolicy.GetNetworkPrefixFromCIDR(route.Dest.BGPRouteState.Network + "/" +
			strconv.Itoa(int(route.Dest.BGPRouteState.CIDRLen)))
		if err != nil {
			s.logger.Info("Invalid route ", ipPrefix)
			continue
		}
		pe.PolicyEngine.PolicyEngineUndoPolicyForEntity(peEntity, policy, callbackInfo)
		pe.DeleteRoutePolicyState(route, policy.Name)
		pe.PolicyEngine.DeletePolicyEntityMapEntry(peEntity, policy.Name)
	}
}

func (s *BGPServer) DoesAdjRIBRouteExist(params interface{}, adjRIBDir bgprib.AdjRIBDir) bool {
	policyParams := params.(*AdjRIBPolicyParams)
	peer := policyParams.Peer
	if peer == nil {
		s.logger.Info("BGPServer:DoesAdjRIBRouteExist - Neighbor not set for route, nlri =",
			policyParams.Route.NLRI.String())
		return false
	}

	adjRIB := s.GetAdjRIB(peer, adjRIBDir)
	if prefixRouteMap, ok := adjRIB[policyParams.Route.ProtocolFamily]; ok {
		if prefixRouteMap[policyParams.Route.NLRI.String()] != nil {
			return true
		}
	}

	return false
}

func (s *BGPServer) DoesAdjRIBInRouteExist(params interface{}) bool {
	return s.DoesAdjRIBRouteExist(params, bgprib.AdjRIBDirIn)
}

func (s *BGPServer) DoesAdjRIBOutRouteExist(params interface{}) bool {
	return s.DoesAdjRIBRouteExist(params, bgprib.AdjRIBDirOut)
}

func (s *BGPServer) ApplyAdjRIBAction(actionInfo interface{}, conditionInfo []interface{}, params interface{}) {
	policyParams := params.(*AdjRIBPolicyParams)
	s.logger.Infof("BGPServer:ApplyAdjRIBAction - policyParams=%+v\n", policyParams)
	policyParams.Accept = Accept
}

func (s *BGPServer) UndoAdjRIBAction(actionInfo interface{}, conditionInfo []interface{}, params interface{},
	policyStmt utilspolicy.PolicyStmt) {
	policyParams := params.(*AdjRIBPolicyParams)
	s.logger.Info("BGPServer:UndoAdjRIBAction - policyParams=%+v\n", policyParams)
	policyParams.Accept = Accept
}

func (s *BGPServer) UpdateAdjRIBRouteAndPolicyDB(policyDetails utilspolicy.PolicyDetails, params interface{},
	pe *bgppolicy.AdjRibPPolicyEngine) {
	var op int
	policyParams := params.(*AdjRIBPolicyParams)

	if policyParams.DeleteType != bgppolicy.Invalid {
		op = bgppolicy.Del
	} else {
		if policyDetails.EntityDeleted == false {
			s.logger.Info("Reject action was not applied, so add this policy to the route")
			op = bgppolicy.Add
			bgppolicy.UpdateAdjRIBRoutePolicyState(policyParams.Route, op, policyDetails.Policy,
				policyDetails.PolicyStmt)
		}
		policyParams.Route.PolicyHitCounter++
	}
	pe.UpdateAdjRIBPolicyRouteMap(policyParams.Route, policyDetails.Policy, op)
}

func (s *BGPServer) UpdateAdjRIBInRouteAndPolicyDB(policyDetails utilspolicy.PolicyDetails, params interface{}) {
	s.UpdateAdjRIBRouteAndPolicyDB(policyDetails, params, s.ribInPE)
}

func (s *BGPServer) UpdateAdjRIBOutRouteAndPolicyDB(policyDetails utilspolicy.PolicyDetails, params interface{}) {
	s.UpdateAdjRIBRouteAndPolicyDB(policyDetails, params, s.ribOutPE)
}

func (s *BGPServer) GetAdjRIB(peer *Peer, adjRIBDir bgprib.AdjRIBDir) map[uint32]map[string]*bgprib.AdjRIBRoute {
	switch adjRIBDir {
	case bgprib.AdjRIBDirIn:
		return peer.ribIn

	case bgprib.AdjRIBDirOut:
		return peer.ribOut
	}
	return nil
}

func (s *BGPServer) TraverseAndApplyAdjRib(data interface{}, updateFunc utilspolicy.PolicyApplyfunc,
	pe *bgppolicy.AdjRibPPolicyEngine, adjRibDir bgprib.AdjRIBDir) {
	s.logger.Infof("BGPServer:TraverseAndApplyAdjRib - start")
	policyInfo := data.(utilspolicy.ApplyPolicyInfo)
	conditionsDB := pe.PolicyEngine.PolicyConditionsDB
	var neighborIP string
	var peer *Peer
	var ok bool

	for _, condition := range policyInfo.Conditions {
		s.logger.Infof("BGPServer:TraverseAndApplyAdjRib - condition:%+v", condition)
		nodeGet := conditionsDB.Get(patriciaDB.Prefix(condition))
		if nodeGet == nil {
			s.logger.Err("Condition", condition, "not defined")
			return
		}
		node := nodeGet.(utilspolicy.PolicyCondition)
		if node.ConditionType == policyCommonDefs.PolicyConditionTypeNeighborMatch {
			neighborIP = node.ConditionInfo.(string)
		}
	}

	if peer, ok = s.PeerMap[neighborIP]; !ok {
		s.logger.Err("Can't apply policy... Neighbor %s not found", neighborIP)
		return
	}

	adjRIB := s.GetAdjRIB(peer, adjRibDir)
	for _, prefixRouteMap := range adjRIB {
		for _, adjRoute := range prefixRouteMap {
			if adjRoute == nil {
				continue
			}

			s.logger.Debugf("Peer %s - NLRI %s policylist %v hit %v before applying create policy",
				adjRoute.NLRI.GetPrefix(), adjRoute.PolicyList, adjRoute.PolicyHitCounter)
			peEntity := utilspolicy.PolicyEngineFilterEntityParams{
				DestNetIp:  adjRoute.NLRI.GetPrefix().String() + "/" + strconv.Itoa(int(adjRoute.NLRI.GetLength())),
				Neighbor:   peer.NeighborConf.RunningConf.NeighborAddress.String(),
				PolicyList: adjRoute.PolicyList,
			}
			callbackInfo := &AdjRIBPolicyParams{
				CreateType: utilspolicy.Invalid,
				DeleteType: utilspolicy.Invalid,
				Peer:       peer,
				Route:      adjRoute,
			}

			updateFunc(peEntity, policyInfo, callbackInfo)
		}
	}
}

func (s *BGPServer) TraverseAndApplyAdjRibIn(data interface{}, updateFunc utilspolicy.PolicyApplyfunc) {
	s.TraverseAndApplyAdjRib(data, updateFunc, s.ribInPE, bgprib.AdjRIBDirIn)
}

func (s *BGPServer) TraverseAndApplyAdjRibOut(data interface{}, updateFunc utilspolicy.PolicyApplyfunc) {
	s.TraverseAndApplyAdjRib(data, updateFunc, s.ribOutPE, bgprib.AdjRIBDirOut)
}

func (s *BGPServer) TraverseAndReverseAdjRIB(policyData interface{}, pe *bgppolicy.AdjRibPPolicyEngine) {
	policy := policyData.(utilspolicy.Policy)
	s.logger.Info("BGPServer:TraverseAndReverseAdjRIB - policy", policy.Name)
	policyExtensions := policy.Extensions.(bgppolicy.AdjRibPolicyExtensions)
	if len(policyExtensions.RouteList) == 0 {
		fmt.Println("No route affected by this policy, so nothing to do")
		return
	}

	var route *bgprib.AdjRIBRoute
	var peer *Peer
	for idx := 0; idx < len(policyExtensions.RouteInfoList); idx++ {
		route = policyExtensions.RouteInfoList[idx]
		peerIP := route.Neighbor.String()
		if peer == nil {
			var ok bool
			if peer, ok = s.PeerMap[peerIP]; ok {
				s.logger.Err("Peer not found for ip", peerIP, "for NLRI", route.NLRI.GetIPPrefix().String())
				continue
			}
		}

		callbackInfo := &AdjRIBPolicyParams{
			CreateType: utilspolicy.Invalid,
			DeleteType: utilspolicy.Invalid,
			Route:      route,
			Peer:       peer,
		}
		peEntity := utilspolicy.PolicyEngineFilterEntityParams{
			DestNetIp: route.NLRI.GetPrefix().String() + "/" + strconv.Itoa(int(route.NLRI.GetLength())),
			Neighbor:  route.Neighbor.String(),
		}

		pe.PolicyEngine.PolicyEngineUndoPolicyForEntity(peEntity, policy, callbackInfo)
		pe.AdjRIBDeleteRoutePolicyState(route, policy.Name)
		pe.PolicyEngine.DeletePolicyEntityMapEntry(peEntity, policy.Name)
	}
}

func (s *BGPServer) TraverseAndReverseAdjRIBIn(policyData interface{}) {
	s.TraverseAndReverseAdjRIB(policyData, s.ribInPE)
}

func (s *BGPServer) TraverseAndReverseAdjRIBOut(policyData interface{}) {
	s.TraverseAndReverseAdjRIB(policyData, s.ribOutPE)
}

func (s *BGPServer) ProcessUpdate(pktInfo *packet.BGPPktSrc) {
	peer, ok := s.PeerMap[pktInfo.Src]
	if !ok {
		s.logger.Err("BgpServer:ProcessUpdate - Peer not found, address:", pktInfo.Src)
		return
	}

	updated, withdrawn, updatedAddPaths := peer.ReceiveUpdate(pktInfo)
	updated, withdrawn, updatedAddPaths = s.CheckForAggregation(updated, withdrawn, updatedAddPaths)
	s.SendUpdate(updated, withdrawn, updatedAddPaths)
}

func (s *BGPServer) convertDestIPToIPPrefix(routes []*config.RouteInfo) map[uint32][]packet.NLRI {
	pfNLRI := make(map[uint32][]packet.NLRI)
	var protoFamily uint32
	for _, r := range routes {
		ip := net.ParseIP(r.IPAddr)
		if ip == nil {
			s.logger.Errf("Connected route %s/%s is not a valid IP", r.IPAddr, r.Mask)
			continue
		}

		if ip.To4() != nil {
			protoFamily = packet.GetProtocolFamily(packet.AfiIP, packet.SafiUnicast)
		} else {
			protoFamily = packet.GetProtocolFamily(packet.AfiIP6, packet.SafiUnicast)
		}

		s.logger.Infof("Connected route: addr %s netmask %s", r.IPAddr, r.Mask)
		if _, ok := pfNLRI[protoFamily]; !ok {
			pfNLRI[protoFamily] = make([]packet.NLRI, 0)
		}

		ipPrefix := packet.ConstructIPPrefix(r.IPAddr, r.Mask)
		pfNLRI[protoFamily] = append(pfNLRI[protoFamily], ipPrefix)
	}
	return pfNLRI
}

func (s *BGPServer) ProcessConnectedRoutes(installedRoutes, withdrawnRoutes []*config.RouteInfo) {
	s.logger.Info("valid routes:", installedRoutes, "invalid routes:", withdrawnRoutes)
	valid := s.convertDestIPToIPPrefix(installedRoutes)
	invalid := s.convertDestIPToIPPrefix(withdrawnRoutes)
	s.logger.Info("pfNLRI valid:", valid, "invalid:", invalid)
	routerId := s.BgpConfig.Global.Config.RouterId.String()
	updated, withdrawn, updatedAddPaths := s.LocRib.ProcessConnectedRoutes(routerId, s.ConnRoutesPath, valid,
		invalid, s.AddPathCount)
	updated, withdrawn, updatedAddPaths = s.CheckForAggregation(updated, withdrawn, updatedAddPaths)
	s.SendUpdate(updated, withdrawn, updatedAddPaths)
}

func (s *BGPServer) ProcessIntfStates(intfs []*config.IntfStateInfo) {
	for _, ifState := range intfs {
		if ifState.State == config.INTF_CREATED {
			s.ifaceMgr.AddIface(ifState.Idx, ifState.IPAddr)
		} else if ifState.State == config.INTF_DELETED {
			s.ifaceMgr.RemoveIface(ifState.Idx, ifState.IPAddr)
		} else if ifState.State == config.IPV6_NEIGHBOR_CREATED {
			s.ifaceMgr.AddLinkLocalIface(ifState.Idx, ifState.LinkLocalIP)
		} else if ifState.State == config.IPV6_NEIGHBOR_DELETED {
			s.ifaceMgr.RemoveLinkLocalIface(ifState.Idx, ifState.LinkLocalIP)
		}
	}
}
func (s *BGPServer) GetIfaceIP(ifIndex int32) (ipInfo utils.IPInfo, err error) {
	ipInfo, err = s.ifaceMgr.GetIfaceIP(ifIndex)
	return ipInfo, err
}
func (s *BGPServer) ProcessRemoveNeighbor(peerIp string, peer *Peer) {
	updated, withdrawn, updatedAddPaths := s.LocRib.RemoveUpdatesFromNeighbor(peerIp, peer.NeighborConf,
		s.AddPathCount)
	s.logger.Infof("ProcessRemoveNeighbor - Neighbor %s, send updated paths %v, withdrawn paths %v",
		peerIp, updated, withdrawn)
	updated, withdrawn, updatedAddPaths = s.CheckForAggregation(updated, withdrawn, updatedAddPaths)
	s.SendUpdate(updated, withdrawn, updatedAddPaths)
}

func (s *BGPServer) SendAllRoutesToPeer(peer *Peer) {
	withdrawn := make([]*bgprib.Destination, 0)
	updatedAddPaths := make([]*bgprib.Destination, 0)
	updated := s.LocRib.GetLocRib()
	s.SendUpdate(updated, withdrawn, updatedAddPaths)
}

func (s *BGPServer) RemoveRoutesFromAllNeighbor() {
	s.LocRib.RemoveUpdatesFromAllNeighbors(s.AddPathCount)
}

func (s *BGPServer) addPeerToList(peer *Peer) {
	s.Neighbors = append(s.Neighbors, peer)
}

func (s *BGPServer) removePeerFromList(peer *Peer) {
	for idx, item := range s.Neighbors {
		if item == peer {
			s.Neighbors[idx] = s.Neighbors[len(s.Neighbors)-1]
			s.Neighbors[len(s.Neighbors)-1] = nil
			s.Neighbors = s.Neighbors[:len(s.Neighbors)-1]
			break
		}
	}
}

func (s *BGPServer) StopPeersByGroup(groupName string) []*Peer {
	peers := make([]*Peer, 0)
	for peerIP, peer := range s.PeerMap {
		if peer.NeighborConf.Group != nil && peer.NeighborConf.Group.Name == groupName {
			s.logger.Info("Clean up peer", peerIP)
			peer.Cleanup()
			s.ProcessRemoveNeighbor(peerIP, peer)
			peers = append(peers, peer)

			runtime.Gosched()
		}
	}

	return peers
}

func (s *BGPServer) UpdatePeerGroupInPeers(groupName string, peerGroup *config.PeerGroupConfig) {
	peers := s.StopPeersByGroup(groupName)
	for _, peer := range peers {
		peer.UpdatePeerGroup(peerGroup)
		peer.Init()
	}
}

func (s *BGPServer) SetupRedistribution(gConf config.GlobalConfig) {
	s.logger.Info("SetUpRedistribution")
	if gConf.Redistribution == nil || len(gConf.Redistribution) == 0 {
		s.logger.Info("No redistribution policies configured")
		return
	}
	conditions := make([]*config.ConditionInfo, 0)
	for i := 0; i < len(gConf.Redistribution); i++ {
		s.logger.Info("Sources: ", gConf.Redistribution[i].Sources)
		sources := make([]string, 0)
		sources = strings.Split(gConf.Redistribution[i].Sources, ",")
		s.logger.Infof("Setting up %s as redistribution policy for source(s): ", gConf.Redistribution[i].Policy)
		for j := 0; j < len(sources); j++ {
			s.logger.Infof("%s ", sources[j])
			if sources[j] == "" {
				continue
			}
			conditions = append(conditions, &config.ConditionInfo{ConditionType: "MatchProtocol", Protocol: sources[j]})
		}
		s.logger.Info("")
		s.routeMgr.ApplyPolicy("BGP", gConf.Redistribution[i].Policy, "Redistribution", conditions)
	}
}

func (s *BGPServer) DeleteAgg(aggConf config.BGPAggregate) error {
	pe, ok := s.locRibPE[aggConf.AddressFamily]
	if ok {
		policyEngine := pe.GetPolicyEngine()
		policyDB := policyEngine.PolicyDB

		nodeGet := policyDB.Get(patriciaDB.Prefix(aggConf.IPPrefix))
		if nodeGet == nil {
			s.logger.Err("Policy ", aggConf, " not created yet")
			return errors.New(fmt.Sprintf("Policy %s not found in policy engine", aggConf.IPPrefix))
		}
		node := nodeGet.(utilspolicy.Policy)

		pe.ReleaseId(uint32(node.Precedence))
		pe.DeletePolicyDefinition(aggConf.IPPrefix)
		pe.DeletePolicyStmt(aggConf.IPPrefix)
		pe.DeletePolicyCondition(aggConf.IPPrefix)
	}
	if _, ok := s.BgpConfig.Afs[aggConf.AddressFamily]; ok {
		delete(s.BgpConfig.Afs[aggConf.AddressFamily].BgpAggs, aggConf.IPPrefix)
	}
	return nil
}

func (s *BGPServer) AddOrUpdateAgg(oldConf config.BGPAggregate, newConf config.BGPAggregate, attrSet []bool) error {
	s.logger.Info("AddOrUpdateAgg")
	var err error

	pe, ok := s.locRibPE[newConf.AddressFamily]
	if !ok {
		s.logger.Err("Aggregate policy engine not created for address family", newConf.AddressFamily)
		return errors.New(fmt.Sprintf("Aggregate policy engine not created for address family", newConf.AddressFamily))
	}

	bytes := packet.GetAddressLengthForFamily(newConf.AddressFamily)
	if bytes == -1 {
		s.logger.Err("Could not find number of bytes for aggregate prefix family", newConf.AddressFamily)
		return errors.New(fmt.Sprintf("Could not find number of bytes for aggregate prefix family",
			newConf.AddressFamily))
	}
	strBits := strconv.Itoa(bytes * 8)
	s.logger.Infof("AddOrUpdateAgg: agg = %s, bytes = %d, bits =%s", newConf.IPPrefix, bytes, strBits)

	if oldConf.IPPrefix != "" {
		// Delete the policy
		s.DeleteAgg(oldConf)
	}

	if newConf.IPPrefix != "" {
		// Create the policy
		name := newConf.IPPrefix
		tokens := strings.Split(newConf.IPPrefix, "/")
		prefixLen := tokens[1]
		_, err := strconv.Atoi(prefixLen)
		if err != nil {
			s.logger.Errf("Failed to convert prefex len %s to int with error %s", prefixLen, err)
			return err
		}

		cond := utilspolicy.PolicyConditionConfig{
			Name:          name,
			ConditionType: "MatchDstIpPrefix",
			MatchDstIpPrefixConditionInfo: utilspolicy.PolicyDstIpMatchPrefixSetCondition{
				Prefix: utilspolicy.PolicyPrefix{
					IpPrefix:        newConf.IPPrefix,
					MasklengthRange: prefixLen + "-" + strBits,
				},
			},
		}

		_, err = pe.CreatePolicyCondition(cond)
		if err != nil {
			s.logger.Errf("Failed to create policy condition for aggregate %s with error %s", name, err)
			return err
		}

		stmt := utilspolicy.PolicyStmtConfig{Name: name, MatchConditions: "all"}
		stmt.Conditions = make([]string, 1)
		stmt.Conditions[0] = name
		stmt.Actions = make([]string, 1)
		stmt.Actions[0] = "permit"
		err = pe.CreatePolicyStmt(stmt)
		if err != nil {
			s.logger.Errf("Failed to create policy statement for aggregate %s with error %s", name, err)
			pe.DeletePolicyCondition(name)
			return err
		}

		precedence := pe.GetNextId()
		def := utilspolicy.PolicyDefinitionConfig{Name: name, Precedence: int(precedence), MatchType: "any"}
		def.PolicyDefinitionStatements = make([]utilspolicy.PolicyDefinitionStmtPrecedence, 1)
		policyDefinitionStatement := utilspolicy.PolicyDefinitionStmtPrecedence{
			Precedence: 1,
			Statement:  name,
		}
		def.PolicyDefinitionStatements[0] = policyDefinitionStatement
		def.Extensions = bgppolicy.PolicyExtensions{}
		err = pe.CreatePolicyDefinition(def)
		if err != nil {
			s.logger.Errf("Failed to create policy definition for aggregate %s with error %s", name, err)
			pe.ReleaseId(precedence)
			pe.DeletePolicyStmt(name)
			pe.DeletePolicyCondition(name)
			return err
		}

		err = s.UpdateAggPolicy(name, pe, newConf)
		if _, ok := s.BgpConfig.Afs[newConf.AddressFamily]; !ok {
			s.BgpConfig.Afs[newConf.AddressFamily] = &config.AddressFamily{}
		}
		if s.BgpConfig.Afs[newConf.AddressFamily].BgpAggs == nil {
			s.BgpConfig.Afs[newConf.AddressFamily].BgpAggs = make(map[string]*config.BGPAggregate)
		}
		s.BgpConfig.Afs[newConf.AddressFamily].BgpAggs[newConf.IPPrefix] = &newConf
		return err
	}
	return err
}

func (s *BGPServer) UpdateAggPolicy(policyName string, pe *bgppolicy.LocRibPolicyEngine,
	aggConf config.BGPAggregate) error {
	s.logger.Debug("UpdateApplyPolicy")
	var err error
	var policyAction utilspolicy.PolicyAction
	conditionNameList := make([]string, 0)

	policyEngine := pe.GetPolicyEngine()
	policyDB := policyEngine.PolicyDB

	nodeGet := policyDB.Get(patriciaDB.Prefix(policyName))
	if nodeGet == nil {
		s.logger.Err("Policy ", policyName, " not defined")
		return errors.New(fmt.Sprintf("Policy %s not found in policy engine", policyName))
	}
	node := nodeGet.(utilspolicy.Policy)

	aggregateActionInfo := utilspolicy.PolicyAggregateActionInfo{aggConf.GenerateASSet, aggConf.SendSummaryOnly}
	policyAction = utilspolicy.PolicyAction{
		Name:       aggConf.IPPrefix,
		ActionType: policyCommonDefs.PolicyActionTypeAggregate,
		ActionInfo: aggregateActionInfo,
	}

	s.logger.Debug("Calling applypolicy with conditionNameList: ", conditionNameList)
	pe.UpdateApplyPolicy(utilspolicy.ApplyPolicyInfo{node, policyAction, conditionNameList}, true)
	return err
}

func (s *BGPServer) copyGlobalConf(gConf config.GlobalConfig) {
	s.BgpConfig.Global.Config.AS = gConf.AS
	s.BgpConfig.Global.Config.RouterId = gConf.RouterId
	s.BgpConfig.Global.Config.UseMultiplePaths = gConf.UseMultiplePaths
	s.BgpConfig.Global.Config.EBGPMaxPaths = gConf.EBGPMaxPaths
	s.BgpConfig.Global.Config.EBGPAllowMultipleAS = gConf.EBGPAllowMultipleAS
	s.BgpConfig.Global.Config.IBGPMaxPaths = gConf.IBGPMaxPaths
}

func (s *BGPServer) handleBfdNotifications(oper config.Operation, DestIp string,
	State bool) {
	if peer, ok := s.PeerMap[DestIp]; ok {
		if !State && peer.NeighborConf.Neighbor.State.BfdNeighborState == "up" {
			peer.NeighborConf.BfdFaultSet()
			peer.Command(int(fsm.BGPEventManualStop), fsm.BGPCmdReasonNone)
		}
		if State && peer.NeighborConf.Neighbor.State.BfdNeighborState == "down" {
			peer.NeighborConf.BfdFaultCleared()
			peer.Command(int(fsm.BGPEventManualStart), fsm.BGPCmdReasonNone)
		}
		s.logger.Info("Bfd state of peer ", peer.NeighborConf.Neighbor.NeighborAddress, " is ",
			peer.NeighborConf.Neighbor.State.BfdNeighborState)
	}
}

func (s *BGPServer) setInterfaceMapForPeer(peerIP string, peer *Peer) {
	s.logger.Info("Server: setInterfaceMapForPeer Peer", peer, "calling GetRouteReachabilityInfo")
	reachInfo, err := s.routeMgr.GetNextHopInfo(peerIP)
	s.logger.Info("Server: setInterfaceMapForPeer Peer", peer, "GetRouteReachabilityInfo returned", reachInfo)
	if err != nil {
		s.logger.Infof("Server: Peer %s is not reachable", peerIP)
	} else {
		// @TODO: jgheewala think of something better for ovsdb....
		ifIdx := s.IntfMgr.GetIfIndex(int(reachInfo.NextHopIfIndex),
			int(reachInfo.NextHopIfType))
		///		ifIdx := asicdCommonDefs.GetIfIndexFromIntfIdAndIntfType(int(reachInfo.NextHopIfIndex), int(reachInfo.NextHopIfType))
		s.logger.Infof("Server: Peer %s IfIdx %d", peerIP, ifIdx)
		if _, ok := s.IfacePeerMap[ifIdx]; !ok {
			s.IfacePeerMap[ifIdx] = make([]string, 0)
		}
		s.IfacePeerMap[ifIdx] = append(s.IfacePeerMap[ifIdx], peerIP)
		peer.setIfIdx(ifIdx)
	}
}

func (s *BGPServer) clearInterfaceMapForPeer(peerIP string, peer *Peer) {
	ifIdx := peer.getIfIdx()
	s.logger.Infof("Server: Peer %s FSM connection broken ifIdx %v", peerIP, ifIdx)
	if peerList, ok := s.IfacePeerMap[ifIdx]; ok {
		for idx, ip := range peerList {
			if ip == peerIP {
				s.IfacePeerMap[ifIdx] = append(s.IfacePeerMap[ifIdx][:idx],
					s.IfacePeerMap[ifIdx][idx+1:]...)
				if len(s.IfacePeerMap[ifIdx]) == 0 {
					delete(s.IfacePeerMap, ifIdx)
				}
				break
			}
		}
	}
	peer.setIfIdx(-1)
}

func (s *BGPServer) constructBGPGlobalState(gConf *config.GlobalConfig) {
	s.BgpConfig.Global.State.AS = gConf.AS
	s.BgpConfig.Global.State.RouterId = gConf.RouterId
	s.BgpConfig.Global.State.UseMultiplePaths = gConf.UseMultiplePaths
	s.BgpConfig.Global.State.EBGPMaxPaths = gConf.EBGPMaxPaths
	s.BgpConfig.Global.State.EBGPAllowMultipleAS = gConf.EBGPAllowMultipleAS
	s.BgpConfig.Global.State.IBGPMaxPaths = gConf.IBGPMaxPaths
}

func (s *BGPServer) listenChannelUpdates() {
	for {
		select {
		case globalUpdate := <-s.GlobalConfigCh:
			for peerIP, peer := range s.PeerMap {
				s.logger.Infof("Cleanup peer %s", peerIP)
				peer.Cleanup()
			}
			s.logger.Infof("Giving up CPU so that all peer FSMs will get cleaned up")
			runtime.Gosched()

			gConf := globalUpdate.NewConfig
			packet.SetNextHopPathAttrs(s.ConnRoutesPath.PathAttrs, gConf.RouterId)
			s.RemoveRoutesFromAllNeighbor()
			s.copyGlobalConf(gConf)
			s.constructBGPGlobalState(&gConf)
			for _, peer := range s.PeerMap {
				peer.Init()
			}
			s.SetupRedistribution(gConf)

		case peerUpdate := <-s.AddPeerCh:
			s.logger.Info("message received on AddPeerCh")
			oldPeer := peerUpdate.OldPeer
			newPeer := peerUpdate.NewPeer
			var peer *Peer
			var ok bool
			if oldPeer.NeighborAddress != nil {
				if peer, ok = s.PeerMap[oldPeer.NeighborAddress.String()]; ok {
					s.logger.Info("Clean up peer", oldPeer.NeighborAddress.String())
					peer.Cleanup()
					s.ProcessRemoveNeighbor(oldPeer.NeighborAddress.String(), peer)
					if peer.NeighborConf.RunningConf.NeighborAddress.To4() != nil &&
						peer.NeighborConf.RunningConf.AuthPassword != "" {
						err := netUtils.SetTCPListenerMD5(s.listener, oldPeer.NeighborAddress.String(), "")
						if err != nil {
							s.logger.Info("Failed to add MD5 authentication for old neighbor",
								newPeer.NeighborAddress.String(), "with error", err)
						}
					}
					peer.UpdateNeighborConf(newPeer, &s.BgpConfig)

					runtime.Gosched()
				} else {
					s.logger.Info("Can't find neighbor with old address", oldPeer.NeighborAddress.String())
				}
			}

			if !ok {
				_, ok = s.PeerMap[newPeer.NeighborAddress.String()]
				if ok {
					s.logger.Info("Failed to add neighbor. Neighbor at that address already exists,",
						newPeer.NeighborAddress.String())
					break
				}

				var groupConfig *config.PeerGroupConfig
				if newPeer.PeerGroup != "" {
					if group, ok :=
						s.BgpConfig.PeerGroups[newPeer.PeerGroup]; !ok {
						s.logger.Info("Peer group", newPeer.PeerGroup, "not created yet, creating peer",
							newPeer.NeighborAddress.String(), "without the group")
					} else {
						groupConfig = &group.Config
					}
				}
				s.logger.Info("Add neighbor, ip:", newPeer.NeighborAddress.String())
				peer = NewPeer(s, s.LocRib, &s.BgpConfig.Global.Config, groupConfig, newPeer)
				if peer.NeighborConf.RunningConf.NeighborAddress.To4() != nil &&
					peer.NeighborConf.RunningConf.AuthPassword != "" {
					err := netUtils.SetTCPListenerMD5(s.listener, newPeer.NeighborAddress.String(),
						peer.NeighborConf.RunningConf.AuthPassword)
					if err != nil {
						s.logger.Info("Failed to add MD5 authentication for neighbor",
							newPeer.NeighborAddress.String(), "with error", err)
					}
				}
				s.PeerMap[newPeer.NeighborAddress.String()] = peer
				s.NeighborMutex.Lock()
				s.addPeerToList(peer)
				s.NeighborMutex.Unlock()
			}
			peer.Init()

		case remPeer := <-s.RemPeerCh:
			s.logger.Info("Remove Peer:", remPeer)
			peer, ok := s.PeerMap[remPeer]
			if !ok {
				s.logger.Info("Failed to remove peer. Peer at that address does not exist,", remPeer)
				break
			}
			s.NeighborMutex.Lock()
			s.removePeerFromList(peer)
			s.NeighborMutex.Unlock()
			delete(s.PeerMap, remPeer)
			peer.Cleanup()
			s.ProcessRemoveNeighbor(remPeer, peer)

		case groupUpdate := <-s.AddPeerGroupCh:
			oldGroupConf := groupUpdate.OldGroup
			newGroupConf := groupUpdate.NewGroup
			s.logger.Info("Peer group update old:", oldGroupConf, "new:", newGroupConf)
			var ok bool

			if oldGroupConf.Name != "" {
				if _, ok = s.BgpConfig.PeerGroups[oldGroupConf.Name]; !ok {
					s.logger.Err("Could not find peer group", oldGroupConf.Name)
					break
				}
			}

			if _, ok = s.BgpConfig.PeerGroups[newGroupConf.Name]; !ok {
				s.logger.Info("Add new peer group with name", newGroupConf.Name)
				peerGroup := config.PeerGroup{
					Config: newGroupConf,
				}
				s.BgpConfig.PeerGroups[newGroupConf.Name] = &peerGroup
			}
			s.UpdatePeerGroupInPeers(newGroupConf.Name, &newGroupConf)

		case groupName := <-s.RemPeerGroupCh:
			s.logger.Info("Remove Peer group:", groupName)
			if _, ok := s.BgpConfig.PeerGroups[groupName]; !ok {
				s.logger.Info("Peer group", groupName, "not found")
				break
			}
			delete(s.BgpConfig.PeerGroups, groupName)
			s.UpdatePeerGroupInPeers(groupName, nil)

		case aggUpdate := <-s.AddAggCh:
			oldAgg := aggUpdate.OldAgg
			newAgg := aggUpdate.NewAgg
			if newAgg.IPPrefix != "" {
				s.AddOrUpdateAgg(oldAgg, newAgg, aggUpdate.AttrSet)
			}

		case aggConf := <-s.RemAggCh:
			s.DeleteAgg(aggConf)

		case tcpConn := <-s.acceptCh:
			s.logger.Info("Connected to", tcpConn.RemoteAddr().String())
			host, _, _ := net.SplitHostPort(tcpConn.RemoteAddr().String())
			peer, ok := s.PeerMap[host]
			if !ok {
				s.logger.Info("Can't accept connection. Peer is not configured yet", host)
				tcpConn.Close()
				s.logger.Info("Closed connection from", host)
				break
			}
			peer.AcceptConn(tcpConn)

		case peerCommand := <-s.PeerCommandCh:
			s.logger.Info("Peer Command received", peerCommand)
			peer, ok := s.PeerMap[peerCommand.IP.String()]
			if !ok {
				s.logger.Infof("Failed to apply command %s. Peer at that address does not exist, %v",
					peerCommand.Command, peerCommand.IP)
			}
			peer.Command(peerCommand.Command, fsm.BGPCmdReasonNone)

		case peerFSMConn := <-s.PeerFSMConnCh:
			s.logger.Infof("Server: Peer %s FSM established/broken channel", peerFSMConn.PeerIP)
			peer, ok := s.PeerMap[peerFSMConn.PeerIP]
			if !ok {
				s.logger.Infof("Failed to process FSM connection success, Peer %s does not exist",
					peerFSMConn.PeerIP)
				break
			}

			if peerFSMConn.Established {
				peer.PeerConnEstablished(peerFSMConn.Conn)
				addPathsMaxTx := peer.getAddPathsMaxTx()
				if addPathsMaxTx > s.AddPathCount {
					s.AddPathCount = addPathsMaxTx
				}
				s.setInterfaceMapForPeer(peerFSMConn.PeerIP, peer)
				s.SendAllRoutesToPeer(peer)
			} else {
				peer.PeerConnBroken(true)
				addPathsMaxTx := peer.getAddPathsMaxTx()
				if addPathsMaxTx < s.AddPathCount {
					s.AddPathCount = 0
					for _, otherPeer := range s.PeerMap {
						addPathsMaxTx = otherPeer.getAddPathsMaxTx()
						if addPathsMaxTx > s.AddPathCount {
							s.AddPathCount = addPathsMaxTx
						}
					}
				}
				s.clearInterfaceMapForPeer(peerFSMConn.PeerIP, peer)
				s.ProcessRemoveNeighbor(peerFSMConn.PeerIP, peer)
			}

		case peerIP := <-s.PeerConnEstCh:
			s.logger.Infof("Server: Peer %s FSM connection established", peerIP)
			peer, ok := s.PeerMap[peerIP]
			if !ok {
				s.logger.Infof("Failed to process FSM connection success, Peer %s does not exist", peerIP)
				break
			}
			reachInfo, err := s.routeMgr.GetNextHopInfo(peerIP)
			if err != nil {
				s.logger.Infof("Server: Peer %s is not reachable", peerIP)
			} else {
				// @TODO: jgheewala think of something better for ovsdb....
				ifIdx := s.IntfMgr.GetIfIndex(int(reachInfo.NextHopIfIndex),
					int(reachInfo.NextHopIfType))
				s.logger.Infof("Server: Peer %s IfIdx %d", peerIP, ifIdx)
				if _, ok := s.IfacePeerMap[ifIdx]; !ok {
					s.IfacePeerMap[ifIdx] = make([]string, 0)
					//ifIdx := asicdCommonDefs.GetIfIndexFromIntfIdAndIntfType(int(reachInfo.NextHopIfIndex), int(reachInfo.NextHopIfType))
				}
				s.IfacePeerMap[ifIdx] = append(s.IfacePeerMap[ifIdx],
					peerIP)
				peer.setIfIdx(ifIdx)
			}

			s.SendAllRoutesToPeer(peer)

		case peerIP := <-s.PeerConnBrokenCh:
			s.logger.Infof("Server: Peer %s FSM connection broken", peerIP)
			peer, ok := s.PeerMap[peerIP]
			if !ok {
				s.logger.Infof("Failed to process FSM connection failure, Peer %s does not exist", peerIP)
				break
			}
			ifIdx := peer.getIfIdx()
			s.logger.Infof("Server: Peer %s FSM connection broken ifIdx %v", peerIP, ifIdx)
			if peerList, ok := s.IfacePeerMap[ifIdx]; ok {
				for idx, ip := range peerList {
					if ip == peerIP {
						s.IfacePeerMap[ifIdx] =
							append(s.IfacePeerMap[ifIdx][:idx],
								s.IfacePeerMap[ifIdx][idx+1:]...)
						if len(s.IfacePeerMap[ifIdx]) == 0 {
							delete(s.IfacePeerMap, ifIdx)
						}
						break
					}
				}
			}
			peer.setIfIdx(-1)
			s.ProcessRemoveNeighbor(peerIP, peer)

		case pktInfo := <-s.BGPPktSrcCh:
			s.logger.Info("Received BGP message from peer %s", pktInfo.Src)
			s.ProcessUpdate(pktInfo)

		case reachabilityInfo := <-s.ReachabilityCh:
			s.logger.Info("Server: Reachability info for ip", reachabilityInfo.IP)

			_, err := s.routeMgr.GetNextHopInfo(reachabilityInfo.IP)
			if err != nil {
				reachabilityInfo.ReachableCh <- false
			} else {
				reachabilityInfo.ReachableCh <- true
			}
		case bfdNotify := <-s.BfdCh:
			s.handleBfdNotifications(bfdNotify.Oper,
				bfdNotify.DestIp, bfdNotify.State)
		case ifState := <-s.IntfCh:
			s.logger.Info("Received message on ItfCh")
			if ifState.State == config.INTF_STATE_DOWN {
				if peerList, ok := s.IfacePeerMap[ifState.Idx]; ok {
					for _, peerIP := range peerList {
						if peer, ok := s.PeerMap[peerIP]; ok {
							peer.StopFSM("Interface Down")
						}
					}
				}
			} else if ifState.State == config.INTF_CREATED {
				s.ifaceMgr.AddIface(ifState.Idx, ifState.IPAddr)
			} else if ifState.State == config.INTF_DELETED {
				s.ifaceMgr.RemoveIface(ifState.Idx, ifState.IPAddr)
			} else if ifState.State == config.IPV6_NEIGHBOR_CREATED {
				s.logger.Info("IPV6_NEIGHBOR_CREATED message")
				s.ifaceMgr.AddLinkLocalIface(ifState.Idx, ifState.LinkLocalIP)
			} else if ifState.State == config.IPV6_NEIGHBOR_DELETED {
				s.ifaceMgr.RemoveLinkLocalIface(ifState.Idx, ifState.LinkLocalIP)
			}
		case ifMap := <-s.IntfMapCh:
			s.logger.Info("Received interface map")
			s.ProcessIntfMapUpdates([]config.IntfMapInfo{config.IntfMapInfo{Idx: ifMap.Idx, IfName: ifMap.IfName}})

		case routeInfo := <-s.RoutesCh:
			s.ProcessConnectedRoutes(routeInfo.Add, routeInfo.Remove)
		}
	}

}
func (s *BGPServer) ProcessIntfMapUpdates(cfg []config.IntfMapInfo) {
	if s.IntfIdNameMap == nil {
		s.IntfIdNameMap = make(map[int32]IntfEntry)
	}
	if s.IfNameToIfIndex == nil {
		s.IfNameToIfIndex = make(map[string]int32)
	}
	for _, ifMap := range cfg {
		intfEntry := IntfEntry{Name: ifMap.IfName}
		s.IntfIdNameMap[int32(ifMap.Idx)] = intfEntry
		s.IfNameToIfIndex[ifMap.IfName] = ifMap.Idx

	}
}
func (s *BGPServer) InitBGPEvent() {
	// Start DB Util
	s.eventDbHdl = dbutils.NewDBUtil(s.logger)
	err := s.eventDbHdl.Connect()
	if err != nil {
		s.logger.Errf("DB connect failed with error %s. Exiting!!", err)
		return
	}
	err = eventUtils.InitEvents("BGPD", s.eventDbHdl, s.eventDbHdl, s.logger, 1000)
	if err != nil {
		s.logger.Err("Unable to initialize events", err)
	}
}
func (s *BGPServer) GetIntfObjects() {

	intfs := s.IntfMgr.GetIPv4Intfs()
	s.ProcessIntfStates(intfs)
	s.logger.Info("After ProcessIntfStates for intfs")

	v6intfs := s.IntfMgr.GetIPv6Intfs()
	s.ProcessIntfStates(v6intfs)
	s.logger.Info("After ProcessIntfStates for v6Intfs")

	v6Neighbors := s.IntfMgr.GetIPv6Neighbors()
	s.ProcessIntfStates(v6Neighbors)
	s.logger.Info("After ProcessIntfStates for v6Neighbors")

	portIntfMap := s.IntfMgr.GetPortInfo()
	s.ProcessIntfMapUpdates(portIntfMap)
	s.logger.Info("After ProcessIntfMapUpdates for ports")

	vlanIntfMap := s.IntfMgr.GetVlanInfo()
	s.ProcessIntfMapUpdates(vlanIntfMap)
	s.logger.Info("After ProcessIntfMapUpdates for vlans")

	logicalIntfMap := s.IntfMgr.GetLogicalIntfInfo()
	s.ProcessIntfMapUpdates(logicalIntfMap)
	s.logger.Info("After ProcessIntfMapUpdates for logicalIntfs")

}

func (s *BGPServer) StartServer() {
	// Initialize Event Handler
	s.InitBGPEvent()
	//read the intfMgr objects before the global conf - this is the case during restart
	s.GetIntfObjects()
	s.ServerUpCh <- true
	s.logger.Info("Setting serverup to true")

	globalUpdate := <-s.GlobalConfigCh
	gConf := globalUpdate.NewConfig
	s.GlobalCfgDone = true
	s.logger.Info("Recieved global conf:", gConf)
	s.BgpConfig.Global.Config = gConf
	s.constructBGPGlobalState(&gConf)
	s.BgpConfig.PeerGroups = make(map[string]*config.PeerGroup)

	pathAttrs := packet.ConstructPathAttrForConnRoutes(gConf.AS)
	protoFamily := packet.GetProtocolFamily(packet.AfiIP6, packet.SafiUnicast)
	ipv6MPReach := packet.ConstructIPv6MPReachNLRIForConnRoutes(protoFamily)
	s.ConnRoutesPath = bgprib.NewPath(s.LocRib, nil, pathAttrs, ipv6MPReach, bgprib.RouteTypeConnected)

	s.logger.Info("Setting up Peer connections")
	// channel for accepting connections
	s.acceptCh = make(chan *net.TCPConn)

	s.listener, _ = s.createListener("tcp4")
	go s.listenForPeers(s.listener, s.acceptCh)

	s.listenerIPv6, _ = s.createListener("tcp6")
	go s.listenForPeers(s.listenerIPv6, s.acceptCh)

	s.logger.Info("Start all managers and initialize API Layer")
	s.IntfMgr.Start()
	s.routeMgr.Start()
	s.bfdMgr.Start()
	s.SetupRedistribution(gConf)

	/*  ALERT: StartServer is a go routine and hence do not have any other go routine where
	 *	   you are making calls to other client. FlexSwitch uses thrift for rpc and hence
	 *	   on return it will not know which go routine initiated the thrift call.
	 */
	// Get routes from the route manager
	add, remove := s.routeMgr.GetRoutes()
	if add != nil && remove != nil {
		s.ProcessConnectedRoutes(add, remove)
	}
	s.GetIntfObjects()
	s.listenChannelUpdates()
}

func (s *BGPServer) GetBGPGlobalState() config.GlobalState {
	return s.BgpConfig.Global.State
}

func (s *BGPServer) GetBGPNeighborState(neighborIP string) *config.NeighborState {
	peer, ok := s.PeerMap[neighborIP]
	if !ok {
		s.logger.Errf("GetBGPNeighborState - Neighbor not found for address:%s", neighborIP)
		return nil
	}
	return &peer.NeighborConf.Neighbor.State
}

func (s *BGPServer) bulkGetBGPNeighbors(index int, count int, addrType config.PeerAddressType) (int, int,
	[]*config.NeighborState) {
	defer s.NeighborMutex.RUnlock()

	s.NeighborMutex.RLock()

	num := 0
	result := make([]*config.NeighborState, 0)
	for i := index; i < len(s.Neighbors); i++ {
		if s.Neighbors[i+index].NeighborConf.Neighbor.Config.PeerAddressType == addrType {
			num++
			if num <= count {
				result = append(result, &s.Neighbors[i+index].NeighborConf.Neighbor.State)
			} else {
				break
			}
		}
	}

	if num > count {
		index += count
	} else {
		index = 0
		count = num
	}
	return index, count, result
}

func (s *BGPServer) BulkGetBGPv4Neighbors(index int, count int) (int, int, []*config.NeighborState) {
	return s.bulkGetBGPNeighbors(index, count, config.PeerAddressV4)
}

func (s *BGPServer) BulkGetBGPv6Neighbors(index int, count int) (int, int, []*config.NeighborState) {
	return s.bulkGetBGPNeighbors(index, count, config.PeerAddressV6)
}

func (s *BGPServer) VerifyBgpGlobalConfig() bool {
	return s.GlobalCfgDone
}

func (s *BGPServer) ConvertIntfStrToIfIndexStr(intfString string) (ifIndex string, err error) {
	if val, err := strconv.Atoi(intfString); err == nil {
		//Verify ifIndex is valid
		s.logger.Info("IfIndex = ", val)
		_, ok := s.IntfIdNameMap[int32(val)]
		if !ok {
			s.logger.Err("Cannot create ip route on a unknown L3 interface")
			return ifIndex, errors.New("Cannot create ip route on a unknown L3 interface")
		}
		ifIndex = intfString
	} else {
		//Verify ifName is valid
		if _, ok := s.IfNameToIfIndex[intfString]; !ok {
			return ifIndex, errors.New("Invalid ifName value")
		}
		ifIndex = strconv.Itoa(int(s.IfNameToIfIndex[intfString]))
	}
	return ifIndex, nil
}
