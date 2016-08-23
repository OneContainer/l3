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
// _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  |
// |  |__   |  |     |  |__   \  V  /     |   (----` \   \/    \/   /  |  | `---|  |----`|  ,----'|  |__|  |
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   |
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  |
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__|
//
package server

import (
	_ "fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"l3/ndp/config"
	"l3/ndp/debug"
	"l3/ndp/packet"
	_ "net"
)

/*
 *	Receive Ndp Packet
 */
func (svr *NDPServer) ReceivedNdpPkts(ifIndex int32) {
	ipPort, _ := svr.L3Port[ifIndex]
	if ipPort.PcapBase.PcapHandle == nil {
		debug.Logger.Err("pcap handler for port:", ipPort.IntfRef, "is not valid. ABORT!!!!")
		return
	}
	src := gopacket.NewPacketSource(ipPort.PcapBase.PcapHandle, layers.LayerTypeEthernet)
	in := src.Packets()
	for {
		select {
		case pkt, ok := <-in:
			if !ok {
				continue
			}
			svr.RxPktCh <- &RxPktInfo{pkt, ipPort.IfIndex}
		case <-ipPort.PcapBase.PcapCtrl:
			ipPort.PcapBase.PcapCtrl <- true
			return
		}
	}
	return
}

/*
 *	StartRxTx      a) Check if entry is present in the map
 *		       b) If no entry create one do the initialization for the entry
 *		       c) Create Pcap Handler & add the entry to up interface slice
 *		       d) Start receiving Packets
 */
func (svr *NDPServer) StartRxTx(ifIndex int32) {
	ipPort, exists := svr.L3Port[ifIndex]
	if !exists {
		// This will copy msg (intRef, ifIndex, ipAddr) into ipPort
		// And also create an entry into the ndpL3IntfStateSlice
		debug.Logger.Err("Failed starting RX/TX for interface which was not created, ifIndex:",
			ifIndex, "is not allowed")
		return
	}

	if ipPort.PcapBase.PcapUsers != 0 {
		// update pcap user and move on
		ipPort.PcapBase.PcapUsers += 1
		svr.L3Port[ifIndex] = ipPort
		debug.Logger.Info("Updating total pcap user for", ipPort.IntfRef, "to", ipPort.PcapBase.PcapUsers)
		debug.Logger.Info("Start receiving packets for ip:", ipPort.IpAddr, "on Port", ipPort.IntfRef)
		return
	}
	// create pcap handler if there is none created right now
	if ipPort.PcapBase.PcapHandle == nil {
		var err error
		ipPort.PcapBase.PcapHandle, err = svr.CreatePcapHandler(ipPort.IntfRef)
		if err != nil {
			return
		}
	}
	// create pcap ctrl channel if not created
	if ipPort.PcapBase.PcapCtrl == nil {
		ipPort.PcapBase.PcapCtrl = make(chan bool)
	}
	ipPort.PcapBase.PcapUsers += 1
	svr.L3Port[ifIndex] = ipPort
	debug.Logger.Info("Start rx/tx for port:", ipPort.IntfRef, "ifIndex:", ipPort.IfIndex, "ip address", ipPort.IpAddr)

	// Spawn go routines for rx & tx
	go svr.ReceivedNdpPkts(ipPort.IfIndex)
	svr.ndpUpL3IntfStateSlice = append(svr.ndpUpL3IntfStateSlice, ifIndex)
	// @TODO:When port comes up are we suppose to send out Neigbor Solicitation or Router Solicitation??
	svr.Packet.SendNAMsg(svr.SwitchMac, ipPort.IpAddr, ipPort.PcapBase.PcapHandle)
}

/*
 *	StopRxTx       a) Check if entry is present in the map
 *		       b) If present then send a ctrl signal to stop receiving packets
 *		       c) block until cleanup is going on
 *		       c) delete the entry from up interface slice
 */
func (svr *NDPServer) StopRxTx(ifIndex int32) {
	ipPort, exists := svr.L3Port[ifIndex]
	if !exists {
		debug.Logger.Err("No entry found for ifIndex:", ifIndex)
		return
	}

	/* The below check is based on following assumptions:
	 *	1) fpPort1 has one ip address, bypass the check and delete pcap
	 *	2) fpPort1 has two ip address
	 *		a) 2003::2/64 	- Global Scope
	 *		b) fe80::123/64 - Link Scope
	 *		In this case we will get two Notification for port down from the chip, one is for
	 *		Global Scope Ip and second is for Link Scope..
	 *		On first Notification NDP will update pcap users and move on. Only when second delete
	 *		notification comes then NDP will delete pcap
	 */
	if ipPort.PcapBase.PcapUsers > 1 {
		ipPort.PcapBase.PcapUsers -= 1
		svr.L3Port[ifIndex] = ipPort
		debug.Logger.Info("Updating total pcap user for", ipPort.IntfRef, "to", ipPort.PcapBase.PcapUsers)
		debug.Logger.Info("Stop receiving packets for ip:", ipPort.IpAddr, "on Port", ipPort.IntfRef)
		return
	}
	// Inform go routine spawned for ipPort to exit..
	ipPort.PcapBase.PcapCtrl <- true
	<-ipPort.PcapBase.PcapCtrl

	// once go routine is exited, delete pcap handler
	svr.DeletePcapHandler(&ipPort.PcapBase.PcapHandle)

	// deleted ctrl channel to avoid any memory usage
	ipPort.PcapBase.PcapCtrl = nil
	ipPort.PcapBase.PcapUsers = 0 // set to zero
	svr.L3Port[ifIndex] = ipPort

	debug.Logger.Info("Stop rx/tx for port:", ipPort.IntfRef, "ifIndex:", ipPort.IfIndex,
		"ip address", ipPort.IpAddr, "is done")
	// Delete Entry from Slice
	svr.DeleteL3IntfFromUpState(ipPort.IfIndex)
}

/*
 *	CheckSrcMac
 *		        a) Check for packet src mac and validate it against ifIndex mac addr
 *			    if it is same then discard the packet
 */
func (svr *NDPServer) CheckSrcMac(macAddr string) bool {
	_, exists := svr.SwitchMacMapEntries[macAddr]
	return exists
}

/*
 *	insertNeighborInfo: Helper API to update list of neighbor keys that are created by ndp
 */
func (svr *NDPServer) insertNeigborInfo(nbrInfo *config.NeighborInfo) {
	svr.NeigborEntryLock.Lock()
	svr.NeighborInfo[nbrInfo.IpAddr] = *nbrInfo
	svr.neighborKey = append(svr.neighborKey, nbrInfo.IpAddr)
	svr.NeigborEntryLock.Unlock()
}

/*
 *	deleteNeighborInfo: Helper API to update list of neighbor keys that are deleted by ndp
 *	@NOTE: caller is responsible for acquiring the lock to access slice
 *	//@TODO: need optimazation here...
 */
func (svr *NDPServer) deleteNeighborInfo(nbrIp string) {
	for idx, _ := range svr.neighborKey {
		if svr.neighborKey[idx] == nbrIp {
			svr.neighborKey = append(svr.neighborKey[:idx],
				svr.neighborKey[idx+1:]...)
			break
		}
	}
}

/*
 *	 CreateNeighborInfo
 *			a) It will first check whether a neighbor exists in the neighbor cache
 *			b) If it doesn't exists then we create neighbor in the platform
 *		        a) It will update ndp server neighbor info cache with the latest information
 */
func (svr *NDPServer) CreateNeighborInfo(nbrInfo *config.NeighborInfo) {
	_, exists := svr.NeighborInfo[nbrInfo.IpAddr]
	if exists {
		return
	}
	debug.Logger.Debug("Calling create ipv6 neighgor for global nbrinfo is", nbrInfo.IpAddr, nbrInfo.MacAddr,
		nbrInfo.VlanId, nbrInfo.IfIndex)
	_, err := svr.SwitchPlugin.CreateIPv6Neighbor(nbrInfo.IpAddr, nbrInfo.MacAddr,
		nbrInfo.VlanId, nbrInfo.IfIndex)
	if err != nil {
		debug.Logger.Err("create ipv6 global neigbor failed for", nbrInfo, "error is", err)
		// do not enter that neighbor in our neigbor map
		return
	}
	svr.SendIPv6CreateNotification(nbrInfo.IpAddr, nbrInfo.IfIndex)
	svr.insertNeigborInfo(nbrInfo)
}

/*
 *	 DeleteNeighborInfo
 *			a) It will first check whether a neighbor exists in the neighbor cache
 *			b) If it doesn't exists then we will move on to next neighbor
 *		        c) If exists then we will call DeleteIPV6Neighbor for that entry and remove
 *			   the entry from our runtime information
 */
func (svr *NDPServer) DeleteNeighborInfo(deleteEntries []string, ifIndex int32) {
	svr.NeigborEntryLock.Lock()
	for _, nbrIp := range deleteEntries {
		nbrEntry, exists := svr.NeighborInfo[nbrIp]
		if !exists {
			debug.Logger.Debug("Neighbor Info for:", nbrIp, "doesn't exists:")
			continue
		}
		debug.Logger.Debug("Calling delete ipv6 neighbor for nbrIp:", nbrEntry.IpAddr)
		_, err := svr.SwitchPlugin.DeleteIPv6Neighbor(nbrEntry.IpAddr)
		if err != nil {
			debug.Logger.Err("delete ipv6 neigbor failed for", nbrEntry, "error is", err)
		}
		svr.deleteNeighborInfo(nbrIp)
		svr.SendIPv6DeleteNotification(nbrIp, ifIndex)
		// delete the entry from neighbor map
		delete(svr.NeighborInfo, nbrIp)
	}
	svr.NeigborEntryLock.Unlock()
}

/*
 *	ProcessRxPkt
 *		        a) Check for runtime information
 *			b) Validate & Parse Pkt, which gives ipAddr, MacAddr
 *			c) PopulateVlanInfo will check if the port is untagged port or not and based of that
 *			   vlan id will be selected
 *			c) CreateIPv6 Neighbor entry
 */
func (svr *NDPServer) ProcessRxPkt(ifIndex int32, pkt gopacket.Packet) {
	_, exists := svr.L3Port[ifIndex]
	if !exists {
		return
	}
	nbrInfo := &config.NeighborInfo{}
	err := svr.Packet.ValidateAndParse(nbrInfo, pkt, ifIndex)
	if err != nil {
		debug.Logger.Err("Validating and parsing Pkt Failed:", err)
		return
	}
	// @ALERT: always overwrite ifIndex when creating neighbor, if ifIndex has reverse map entry for
	//	   vlanID then that will be overwritten again with vlan ifIndex
	nbrInfo.IfIndex = ifIndex
	if nbrInfo.PktOperation == byte(packet.PACKET_DROP) {
		debug.Logger.Err("Dropping message as PktOperation is PACKET_DROP for", nbrInfo.IpAddr)
		return
	} else if nbrInfo.State == packet.INCOMPLETE {
		debug.Logger.Err("Received message but packet state is INCOMPLETE hence not calling create ipv6 neighbor for ",
			nbrInfo.IpAddr)
		return
	} else if nbrInfo.State == packet.REACHABLE {
		switchMac := svr.CheckSrcMac(nbrInfo.MacAddr)
		if switchMac {
			debug.Logger.Info("Received Packet from same port and hence ignoring the packet:",
				nbrInfo)
			return
		}
		svr.PopulateVlanInfo(nbrInfo, ifIndex)
		svr.CreateNeighborInfo(nbrInfo)
	} else {
		debug.Logger.Alert("Handle state", nbrInfo.State, "after packet validation & parsing")
	}
	return
}

func (svr *NDPServer) ProcessTimerExpiry(pktData config.PacketData) {
	l3Port, exists := svr.L3Port[pktData.IfIndex]
	if !exists {
		return
	}
	retry := svr.Packet.RetryUnicastSolicitation(pktData.IpAddr, pktData.NeighborIp, l3Port.PcapBase.PcapHandle)
	//if retry {
	// use pktData.IpAddr because that will be your src ip without CIDR format, same goes for NeighborIP
	//	svr.Packet.SendUnicastNeighborSolicitation(pktData.IpAddr, pktData.NeighborIp, l3Port.PcapBase.PcapHandle)
	//} else {
	if !retry {
		// delete single Neighbor entry from Neighbor Cache
		deleteEntries, err := svr.Packet.DeleteNeighbor(pktData.IpAddr, pktData.NeighborIp)
		if len(deleteEntries) > 0 && err == nil {
			svr.DeleteNeighborInfo(deleteEntries, pktData.IfIndex)
		}
	}
}
