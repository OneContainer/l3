package server

import (
	"fmt"
	"l3/ospf/config"
	"time"
)

type LsdbUpdateMsg struct {
	MsgType uint8
	AreaId  uint32
	Data    []byte
}

func NewLsdbUpdateMsg() *LsdbUpdateMsg {
	return &LsdbUpdateMsg{}
}

type LSAChangeMsg struct {
	areaId uint32
}

type NetworkLSAChangeMsg struct {
	areaId  uint32
	intfKey IntfConfKey
}

const (
	LsdbAdd      uint8 = 0
	LsdbDel      uint8 = 1
	LsdbUpdate   uint8 = 2
	LsdbNoAction uint8 = 3
)

const (
	P2PLink     uint8 = 1
	TransitLink uint8 = 2
	StubLink    uint8 = 3
	VirtualLink uint8 = 4
)

func (server *OSPFServer) initLSDatabase(areaId uint32) {
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		lsDbEnt.RouterLsaMap = make(map[LsaKey]RouterLsa)
		lsDbEnt.NetworkLsaMap = make(map[LsaKey]NetworkLsa)
		lsDbEnt.Summary3LsaMap = make(map[LsaKey]SummaryLsa)
		lsDbEnt.Summary4LsaMap = make(map[LsaKey]SummaryLsa)
		lsDbEnt.ASExternalLsaMap = make(map[LsaKey]ASExternalLsa)
		server.AreaLsdb[lsdbKey] = lsDbEnt
	}
	selfOrigLsaEnt, exist := server.AreaSelfOrigLsa[lsdbKey]
	if !exist {
		selfOrigLsaEnt = make(map[LsaKey]bool)
		server.AreaSelfOrigLsa[lsdbKey] = selfOrigLsaEnt
	}
}

func (server *OSPFServer) lsdbStateRefresh() {
	var lsdbStateRefFunc func()
	lsdbStateRefFunc = func() {
		server.logger.Info("Inside lsdbStateRefFunc()")
		server.logger.Info(fmt.Sprintln("The old Lsdb Slice after refresh", server.LsdbSlice))
		server.LsdbSlice = []LsdbSliceEnt{}
		for lsdbkey, lsdbEnt := range server.AreaLsdb {
			for lsakey, _ := range lsdbEnt.RouterLsaMap {
				var val LsdbSliceEnt
				val.AreaId = lsdbkey.AreaId
				val.LSType = lsakey.LSType
				val.LSId = lsakey.LSId
				val.AdvRtr = lsakey.AdvRouter
				server.LsdbSlice = append(server.LsdbSlice, val)
			}
			for lsakey, _ := range lsdbEnt.NetworkLsaMap {
				var val LsdbSliceEnt
				val.AreaId = lsdbkey.AreaId
				val.LSType = lsakey.LSType
				val.LSId = lsakey.LSId
				val.AdvRtr = lsakey.AdvRouter
				server.LsdbSlice = append(server.LsdbSlice, val)
			}
			for lsakey, _ := range lsdbEnt.Summary3LsaMap {
				var val LsdbSliceEnt
				val.AreaId = lsdbkey.AreaId
				val.LSType = lsakey.LSType
				val.LSId = lsakey.LSId
				val.AdvRtr = lsakey.AdvRouter
				server.LsdbSlice = append(server.LsdbSlice, val)
			}
			for lsakey, _ := range lsdbEnt.Summary4LsaMap {
				var val LsdbSliceEnt
				val.AreaId = lsdbkey.AreaId
				val.LSType = lsakey.LSType
				val.LSId = lsakey.LSId
				val.AdvRtr = lsakey.AdvRouter
				server.LsdbSlice = append(server.LsdbSlice, val)
			}
			for lsakey, _ := range lsdbEnt.ASExternalLsaMap {
				var val LsdbSliceEnt
				val.AreaId = lsdbkey.AreaId
				val.LSType = lsakey.LSType
				val.LSId = lsakey.LSId
				val.AdvRtr = lsakey.AdvRouter
				server.LsdbSlice = append(server.LsdbSlice, val)
			}
		}
		server.logger.Info(fmt.Sprintln("The new Lsdb Slice after refresh", server.LsdbSlice))
		server.LsdbStateTimer.Reset(server.RefreshDuration)
	}
	server.LsdbStateTimer = time.AfterFunc(server.RefreshDuration, lsdbStateRefFunc)
}

func (server *OSPFServer) StartLSDatabase() {
	server.logger.Info("Initializing LSA Database")
	for key, _ := range server.AreaConfMap {
		areaId := convertAreaOrRouterIdUint32(string(key.AreaId))
		server.initLSDatabase(areaId)
	}

	server.lsdbStateRefresh()
	go server.processLSDatabaseUpdates()
	return
}

func (server *OSPFServer) StopLSDatabase() {

}

func (server *OSPFServer) flushNetworkLSA(areaId uint32, key IntfConfKey) {
	ent := server.IntfConfMap[key]
	AreaId := convertIPv4ToUint32(ent.IfAreaId)
	if areaId != AreaId {
		return
	}
	if ent.IfFSMState <= config.Waiting {
		return
	}

	LSType := NetworkLSA
	LSId := convertAreaOrRouterIdUint32(ent.IfIpAddr.String())
	AdvRouter := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	lsaKey := LsaKey{
		LSType:    LSType,
		LSId:      LSId,
		AdvRouter: AdvRouter,
	}
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]

	// Need to Flush these entries
	delete(lsDbEnt.NetworkLsaMap, lsaKey)
	delete(selfOrigLsaEnt, lsaKey)
	server.AreaSelfOrigLsa[lsdbKey] = selfOrigLsaEnt
	server.AreaLsdb[lsdbKey] = lsDbEnt
}

func (server *OSPFServer) generateNetworkLSA(areaId uint32, key IntfConfKey, isDR bool, nbr_list []uint32) {
	server.logger.Info(fmt.Sprintln("Calling generate Network LSA func", nbr_list))

	//routerId := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	ent := server.IntfConfMap[key]
	AreaId := convertIPv4ToUint32(ent.IfAreaId)
	nbrListLen := len(nbr_list)

	if areaId != AreaId {
		return
	}
	if ent.IfFSMState <= config.Waiting {
		return
	}

	if !isDR {
		server.sendLsdbToNeighborEvent(nbr_list[nbrListLen-1], areaId, 0,
			0, LSAFLOOD)
		return
	}

	netmask := convertIPv4ToUint32(ent.IfNetmask)
	attachedRtr := make([]uint32, 0)
	/*for key, nbrEnt := range ent.NeighborMap {
		if nbrEnt.FullState == false {
			continue
		}
		attachedRtr = append(attachedRtr, key.RouterId)
	}*/
	for index := range nbr_list {
		/* TODO: HACK Need to be removed */
		flag := false
		for i := 0; i < len(attachedRtr); i++ {
			if nbr_list[index] == attachedRtr[i] {
				flag = true
			}
		}
		if flag == false {
			attachedRtr = append(attachedRtr, nbr_list[index])
		}
	}
	selfRtrId := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	attachedRtr = append(attachedRtr, selfRtrId)

	numOfAttachedRtr := len(attachedRtr)
	if numOfAttachedRtr == 0 {
		return
	}

	LSType := NetworkLSA
	LSId := convertAreaOrRouterIdUint32(ent.IfIpAddr.String())
	Options := uint8(2) // Need to be revisited
	LSAge := 0
	AdvRouter := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	lsaKey := LsaKey{
		LSType:    LSType,
		LSId:      LSId,
		AdvRouter: AdvRouter,
	}

	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]
	entry, exist := lsDbEnt.NetworkLsaMap[lsaKey]
	entry.LsaMd.LSAge = 0
	entry.LsaMd.Options = Options
	if !exist {
		entry.LsaMd.LSSequenceNum = InitialSequenceNumber
	} else {
		entry.LsaMd.LSSequenceNum = entry.LsaMd.LSSequenceNum + 1
	}
	entry.LsaMd.LSChecksum = 0
	// Length of Network LSA Metadata (netmask)  = 4 bytes
	entry.LsaMd.LSLen = uint16(OSPF_LSA_HEADER_SIZE + 4 + (4 * numOfAttachedRtr))
	entry.Netmask = netmask
	entry.AttachedRtr = make([]uint32, numOfAttachedRtr)
	for i := 0; i < numOfAttachedRtr; i++ {
		entry.AttachedRtr[i] = attachedRtr[i]
	}
	server.logger.Info(fmt.Sprintln("Attached Routers:", entry.AttachedRtr))
	selfOrigLsaEnt[lsaKey] = true
	server.AreaSelfOrigLsa[lsdbKey] = selfOrigLsaEnt
	server.logger.Info(fmt.Sprintln("Self Originated Router LSA Key:", server.AreaSelfOrigLsa[lsdbKey]))
	LsaEnc := encodeNetworkLsa(entry, lsaKey)
	checksumOffset := uint16(14)
	entry.LsaMd.LSChecksum = computeFletcherChecksum(LsaEnc[2:], checksumOffset)
	entry.LsaMd.LSAge = uint16(LSAge)
	lsDbEnt.NetworkLsaMap[lsaKey] = entry
	server.AreaLsdb[lsdbKey] = lsDbEnt

	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsaKey.LSType
		val.LSId = lsaKey.LSId
		val.AdvRtr = lsaKey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}
	server.sendLsdbToNeighborEvent(nbr_list[nbrListLen-1], areaId, 0,
		0, LSAFLOOD)
	return
}

func (server *OSPFServer) generateRouterLSA(areaId uint32) {
	var linkDetails []LinkDetail = nil
	for _, ent := range server.IntfConfMap {
		AreaId := convertIPv4ToUint32(ent.IfAreaId)
		if areaId != AreaId {
			continue
		}
		if ent.IfFSMState <= config.Waiting {
			continue
		}
		var linkDetail LinkDetail
		if ent.IfType == config.Broadcast {
			if len(ent.NeighborMap) == 0 { // Stub Network
				server.logger.Info("Stub Network")
				ipAddr := convertAreaOrRouterIdUint32(ent.IfIpAddr.String())
				netmask := convertIPv4ToUint32(ent.IfNetmask)
				linkDetail.LinkId = ipAddr & netmask
				/* For links to stub networks, this field specifies the stub
				   network’s IP address mask. */
				linkDetail.LinkData = netmask
				linkDetail.LinkType = StubLink
				/* Todo: Need to handle IfMetricConf */
				linkDetail.NumOfTOS = 0
				linkDetail.LinkMetric = 10
			} else { // Transit Network
				server.logger.Info("Transit Network")
				linkDetail.LinkId = convertIPv4ToUint32(ent.IfDRIp)
				/* For links to transit networks, numbered point-to-point links
				   and virtual links, this field specifies the IP interface
				   address of the associated router interface*/
				linkDetail.LinkData = convertAreaOrRouterIdUint32(ent.IfIpAddr.String())
				linkDetail.LinkType = TransitLink
				/* Todo: Need to handle IfMetricConf */
				linkDetail.NumOfTOS = 0
				linkDetail.LinkMetric = 10
			}
		} else if ent.IfType == config.PointToPoint {
			// linkDetial.LinkId = NBRs Router ID
		}
		linkDetails = append(linkDetails, linkDetail)
	}

	numOfLinks := len(linkDetails)

	LSType := RouterLSA
	LSId := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	Options := uint8(2) // Need to be revisited
	LSAge := 0
	AdvRouter := convertIPv4ToUint32(server.ospfGlobalConf.RouterId)
	BitE := false //not an AS boundary router (Todo)
	BitB := false //not an Area Border Router (Todo)
	lsaKey := LsaKey{
		LSType:    LSType,
		LSId:      LSId,
		AdvRouter: AdvRouter,
	}

	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]

	if numOfLinks == 0 {
		delete(lsDbEnt.RouterLsaMap, lsaKey)
		delete(selfOrigLsaEnt, lsaKey)
		server.AreaSelfOrigLsa[lsdbKey] = selfOrigLsaEnt
		server.AreaLsdb[lsdbKey] = lsDbEnt
		return
	}
	ent, exist := lsDbEnt.RouterLsaMap[lsaKey]
	ent.LsaMd.LSAge = 0
	ent.LsaMd.Options = Options
	if !exist {
		ent.LsaMd.LSSequenceNum = InitialSequenceNumber
	} else {
		ent.LsaMd.LSSequenceNum = ent.LsaMd.LSSequenceNum + 1
	}
	ent.LsaMd.LSChecksum = 0
	// Length of Per Link Details = 12 bytes
	// Length of Router LSA Metadata (BitE, BitB, NumofLinks)  = 4 bytes
	ent.LsaMd.LSLen = uint16(OSPF_LSA_HEADER_SIZE + 4 + (12 * numOfLinks))
	ent.BitE = BitE
	ent.BitB = BitB
	ent.NumOfLinks = uint16(numOfLinks)
	ent.LinkDetails = make([]LinkDetail, numOfLinks)
	copy(ent.LinkDetails, linkDetails[0:])
	server.logger.Info(fmt.Sprintln("LinkDetails:", ent.LinkDetails))
	selfOrigLsaEnt[lsaKey] = true
	server.AreaSelfOrigLsa[lsdbKey] = selfOrigLsaEnt
	server.logger.Info(fmt.Sprintln("Self Originated Router LSA Key:", server.AreaSelfOrigLsa[lsdbKey]))
	LsaEnc := encodeRouterLsa(ent, lsaKey)
	checksumOffset := uint16(14)
	ent.LsaMd.LSChecksum = computeFletcherChecksum(LsaEnc[2:], checksumOffset)
	ent.LsaMd.LSAge = uint16(LSAge)
	lsDbEnt.RouterLsaMap[lsaKey] = ent
	server.AreaLsdb[lsdbKey] = lsDbEnt
	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsaKey.LSType
		val.LSId = lsaKey.LSId
		val.AdvRtr = lsaKey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}
	return
}

func (server *OSPFServer) processDeleteRouterLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	routerLsa := NewRouterLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeRouterLsa(data, routerLsa, lsakey)
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	delete(lsDbEnt.RouterLsaMap, *lsakey)
	server.AreaLsdb[lsdbKey] = lsDbEnt
	return true
}

func (server *OSPFServer) processRecvdRouterLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	routerLsa := NewRouterLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeRouterLsa(data, routerLsa, lsakey)
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]
	_, exist := selfOrigLsaEnt[*lsakey]
	if exist {
		server.logger.Info("Recvd a self generated Router LSA")
		return false
	}
	//Check Checksum
	csum := computeFletcherChecksum(data[2:], FLETCHER_CHECKSUM_VALIDATE)
	if csum != 0 {
		server.logger.Err("Invalid Router LSA Checksum")
		return false
	}
	//Todo: If there is already existing entry Verify the seq num
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	ent, exist := lsDbEnt.RouterLsaMap[*lsakey]
	if exist {
		if ent.LsaMd.LSSequenceNum >= routerLsa.LsaMd.LSSequenceNum {
			server.logger.Err("Old instance of Router LSA Recvd")
			return false
		}
	}
	//Handle LsaAge
	//Add entry in LSADatabase
	lsDbEnt.RouterLsaMap[*lsakey] = *routerLsa
	server.AreaLsdb[lsdbKey] = lsDbEnt
	server.printRouterLsa()
	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsakey.LSType
		val.LSId = lsakey.LSId
		val.AdvRtr = lsakey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}

	return true
}

func (server *OSPFServer) printRouterLsa() {
	server.logger.Info("AREA  LSDB")
	for key, val := range server.AreaLsdb {
		server.logger.Info(fmt.Sprintln("key ", key, " LSA ", val))
	}
}

func (server *OSPFServer) processDeleteNetworkLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	networkLsa := NewNetworkLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeNetworkLsa(data, networkLsa, lsakey)
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	delete(lsDbEnt.NetworkLsaMap, *lsakey)
	server.AreaLsdb[lsdbKey] = lsDbEnt

	return true
}

func (server *OSPFServer) processRecvdNetworkLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	networkLsa := NewNetworkLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeNetworkLsa(data, networkLsa, lsakey)
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]
	_, exist := selfOrigLsaEnt[*lsakey]
	if exist {
		server.logger.Info("Recvd a self generated Network LSA")
		return false
	}

	//Check Checksum
	csum := computeFletcherChecksum(data[2:], FLETCHER_CHECKSUM_VALIDATE)
	if csum != 0 {
		server.logger.Err("Invalid Network LSA Checksum")
		return false
	}
	//Todo: If there is already existing entry Verify the seq num
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	ent, exist := lsDbEnt.NetworkLsaMap[*lsakey]
	if exist {
		if ent.LsaMd.LSSequenceNum >= networkLsa.LsaMd.LSSequenceNum {
			server.logger.Err("Old instance of Network LSA Recvd")
			return false
		}
	}
	//Handle LsaAge
	//Add entry in LSADatabase
	lsDbEnt.NetworkLsaMap[*lsakey] = *networkLsa
	server.AreaLsdb[lsdbKey] = lsDbEnt
	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsakey.LSType
		val.LSId = lsakey.LSId
		val.AdvRtr = lsakey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}
	return true
}

func (server *OSPFServer) processDeleteSummaryLsa(data []byte, areaId uint32, lsaType uint8) bool {
	lsakey := NewLsaKey()
	summaryLsa := NewSummaryLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeSummaryLsa(data, summaryLsa, lsakey)
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	if lsaType == Summary3LSA {
		delete(lsDbEnt.Summary3LsaMap, *lsakey)
	} else if lsaType == Summary4LSA {
		delete(lsDbEnt.Summary4LsaMap, *lsakey)
	}
	server.AreaLsdb[lsdbKey] = lsDbEnt
	server.printRouterLsa()
	return true
}

func (server *OSPFServer) processRecvdSummaryLsa(data []byte, areaId uint32, lsaType uint8) bool {
	lsakey := NewLsaKey()
	summaryLsa := NewSummaryLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeSummaryLsa(data, summaryLsa, lsakey)

	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]
	_, exist := selfOrigLsaEnt[*lsakey]
	if exist {
		server.logger.Info("Recvd a self generated Summary LSA")
		return false
	}

	//Check Checksum
	csum := computeFletcherChecksum(data[2:], FLETCHER_CHECKSUM_VALIDATE)
	if csum != 0 {
		server.logger.Err("Invalid Summary LSA Checksum")
		return false
	}
	//Todo: If there is already existing entry Verify the seq num
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	if lsaType == Summary3LSA {
		ent, exist := lsDbEnt.Summary3LsaMap[*lsakey]
		if exist {
			if ent.LsaMd.LSSequenceNum >= summaryLsa.LsaMd.LSSequenceNum {
				server.logger.Err("Old instance of Summary 3 LSA Recvd")
				return false
			}
		}
		//Handle LsaAge
		//Add entry in LSADatabase
		lsDbEnt.Summary3LsaMap[*lsakey] = *summaryLsa
	} else if lsaType == Summary4LSA {
		ent, exist := lsDbEnt.Summary4LsaMap[*lsakey]
		if exist {
			if ent.LsaMd.LSSequenceNum >= summaryLsa.LsaMd.LSSequenceNum {
				server.logger.Err("Old instance of Summary 4 LSA Recvd")
				return false
			}
		}
		//Handle LsaAge
		//Add entry in LSADatabase
		lsDbEnt.Summary4LsaMap[*lsakey] = *summaryLsa
	} else {
		return false
	}
	server.AreaLsdb[lsdbKey] = lsDbEnt
	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsakey.LSType
		val.LSId = lsakey.LSId
		val.AdvRtr = lsakey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}
	return true
}

func (server *OSPFServer) processDeleteASExternalLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	asExtLsa := NewASExternalLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeASExternalLsa(data, asExtLsa, lsakey)
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	delete(lsDbEnt.ASExternalLsaMap, *lsakey)
	server.AreaLsdb[lsdbKey] = lsDbEnt

	return true
}

func (server *OSPFServer) processRecvdASExternalLsa(data []byte, areaId uint32) bool {
	lsakey := NewLsaKey()
	asExtLsa := NewASExternalLsa()
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	decodeASExternalLsa(data, asExtLsa, lsakey)
	selfOrigLsaEnt, _ := server.AreaSelfOrigLsa[lsdbKey]
	_, exist := selfOrigLsaEnt[*lsakey]
	if exist {
		server.logger.Info("Recvd a self generated AS External LSA")
		return false
	}

	//Check Checksum
	csum := computeFletcherChecksum(data[2:], FLETCHER_CHECKSUM_VALIDATE)
	if csum != 0 {
		server.logger.Err("Invalid AS External LSA Checksum")
		return false
	}
	//Todo: If there is already existing entry Verify the seq num
	lsDbEnt, _ := server.AreaLsdb[lsdbKey]
	ent, exist := lsDbEnt.ASExternalLsaMap[*lsakey]
	if exist {
		if ent.LsaMd.LSSequenceNum >= asExtLsa.LsaMd.LSSequenceNum {
			server.logger.Err("Old instance of AS External LSA Recvd")
			return false
		}
	}
	//Handle LsaAge
	//Add entry in LSADatabase
	lsDbEnt.ASExternalLsaMap[*lsakey] = *asExtLsa
	server.AreaLsdb[lsdbKey] = lsDbEnt
	if !exist {
		var val LsdbSliceEnt
		val.AreaId = lsdbKey.AreaId
		val.LSType = lsakey.LSType
		val.LSId = lsakey.LSId
		val.AdvRtr = lsakey.AdvRouter
		server.LsdbSlice = append(server.LsdbSlice, val)
	}

	return true
}

func (server *OSPFServer) processRecvdLsa(data []byte, areaId uint32) bool {
	LSType := uint8(data[3])
	if LSType == RouterLSA {
		server.logger.Info("LSDB: Received router lsa")
		return server.processRecvdRouterLsa(data, areaId)
	} else if LSType == NetworkLSA {
		server.logger.Info("LSDB: Received network lsa")
		return server.processRecvdNetworkLsa(data, areaId)
	} else if LSType == Summary3LSA {
		server.logger.Info("LSDB: Received summary3 lsa")
		return server.processRecvdSummaryLsa(data, areaId, LSType)
	} else if LSType == Summary4LSA {
		server.logger.Info("LSDB: Received summary4 lsa")
		return server.processRecvdSummaryLsa(data, areaId, LSType)
	} else if LSType == ASExternalLSA {
		return server.processRecvdASExternalLsa(data, areaId)
	} else {
		server.logger.Info("LSDB: Invalid LSA packet from nbr")
		return false
	}
}

func (server *OSPFServer) processDeleteLsa(data []byte, areaId uint32) bool {
	LSType := uint8(data[3])

	if LSType == RouterLSA {
		return server.processDeleteRouterLsa(data, areaId)
	} else if LSType == NetworkLSA {
		return server.processDeleteNetworkLsa(data, areaId)
	} else if LSType == Summary3LSA {
		return server.processDeleteSummaryLsa(data, areaId, LSType)
	} else if LSType == Summary4LSA {
		return server.processDeleteSummaryLsa(data, areaId, LSType)
	} else if LSType == ASExternalLSA {
		return server.processDeleteASExternalLsa(data, areaId)
	} else {
		return false
	}
}

func (server *OSPFServer) processLSDatabaseUpdates() {
	for {
		select {
		case msg := <-server.LsdbUpdateCh:
			if msg.MsgType == LsdbAdd {
				server.logger.Info("Adding LS in the Lsdb")
				server.logger.Info("Received New LSA")
				ret := server.processRecvdLsa(msg.Data, msg.AreaId)
				server.logger.Info(fmt.Sprintln("Return Code:", ret))
				//server.LsaUpdateRetCodeCh <- ret
				server.StartCalcSPFCh <- true
				spfStatus := <-server.DoneCalcSPFCh
				server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
			} else if msg.MsgType == LsdbDel {
				server.logger.Info("Deleting LS in the Lsdb")
				ret := server.processDeleteLsa(msg.Data, msg.AreaId)
				//server.LsaUpdateRetCodeCh <- ret
				server.logger.Info(fmt.Sprintln("Return Code:", ret))
				server.StartCalcSPFCh <- true
				spfStatus := <-server.DoneCalcSPFCh
				server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
			} else if msg.MsgType == LsdbUpdate {
				server.logger.Info("Deleting LS in the Lsdb")
				ret := server.processRecvdLsa(msg.Data, msg.AreaId)
				//server.LsaUpdateRetCodeCh <- ret
				server.logger.Info(fmt.Sprintln("Return Code:", ret))
				server.StartCalcSPFCh <- true
				spfStatus := <-server.DoneCalcSPFCh
				server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
			}
		case msg := <-server.IntfStateChangeCh:
			server.logger.Info(fmt.Sprintf("Interface State change msg", msg))
			server.generateRouterLSA(msg.areaId)
			server.logger.Info(fmt.Sprintln("LS Database", server.AreaLsdb))
			server.StartCalcSPFCh <- true
			spfStatus := <-server.DoneCalcSPFCh
			server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
		case msg := <-server.NetworkDRChangeCh:
			server.logger.Info(fmt.Sprintf("Network DR change msg", msg))
			// Create a new router LSA
			server.generateRouterLSA(msg.areaId)
			server.logger.Info(fmt.Sprintln("LS Database", server.AreaLsdb))
			server.StartCalcSPFCh <- true
			spfStatus := <-server.DoneCalcSPFCh
			server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
		case msg := <-server.CreateNetworkLSACh:
			server.logger.Info(fmt.Sprintf("Create Network LSA msg", msg))
			server.generateNetworkLSA(msg.areaId, msg.intf, msg.isDR, msg.nbrList)
			// Flush the old Network LSA
			// Check if link is broadcast or not
			// If link is broadcast
			// Create Network LSA
			server.logger.Info(fmt.Sprintln("LS Database", server.AreaLsdb))
			server.StartCalcSPFCh <- true
			spfStatus := <-server.DoneCalcSPFCh
			server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
		case msg := <-server.FlushNetworkLSACh:
			server.logger.Info(fmt.Sprintf("Flush Network LSA msg", msg))
			// Flush the old Network LSA
			server.flushNetworkLSA(msg.areaId, msg.intfKey)
			server.logger.Info(fmt.Sprintln("LS Database", server.AreaLsdb))
			server.StartCalcSPFCh <- true
			spfStatus := <-server.DoneCalcSPFCh
			server.logger.Info(fmt.Sprintln("SPF Calculation Return Status", spfStatus))
		}
	}
}
