package server

import (
	"errors"
	"fmt"
	"l3/ospf/config"
)

type VertexKey struct {
	Type   uint8
	ID     uint32
	AdvRtr uint32
}

type Path []VertexKey

type TreeVertex struct {
	Paths      []Path
	Distance   uint16
	NumOfPaths int
}

type StubVertex struct {
	NbrVertexKey  VertexKey
	NbrVertexCost uint16
	LinkData      uint32
	LsaKey        LsaKey
	AreaId        uint32
	LinkStateId   uint32
}

type Vertex struct {
	NbrVertexKey  []VertexKey
	NbrVertexCost []uint16
	LinkData      map[VertexKey]uint32
	LsaKey        LsaKey
	AreaId        uint32
	Visited       bool
	LinkStateId   uint32
	NetMask       uint32
}

const (
	RouterVertex   uint8 = 0
	SNetworkVertex uint8 = 1 // Stub
	TNetworkVertex uint8 = 2 // Transit
)

var check bool = true

func findSelfOrigRouterLsaKey(ent map[LsaKey]bool) (LsaKey, error) {
	var key LsaKey
	for key, _ := range ent {
		if key.LSType == RouterLSA {
			return key, nil
		}
	}
	err := errors.New("No Self Orignated Router LSA found")
	return key, err
}

func (server *OSPFServer) UpdateAreaGraphNetworkLsa(lsaEnt NetworkLsa, lsaKey LsaKey, areaId uint32) error {
	server.logger.Info(fmt.Sprintln("2: Using Lsa with key as:", dumpLsaKey(lsaKey), "for SPF calc"))
	vertexKey := VertexKey{
		Type:   TNetworkVertex,
		ID:     lsaKey.LSId,
		AdvRtr: lsaKey.AdvRouter,
	}
	ent, exist := server.AreaGraph[vertexKey]
	if exist {
		server.logger.Info(fmt.Sprintln("Entry already exists in SPF Graph for vertexKey:", vertexKey))
		server.logger.Info(fmt.Sprintln("SPF Graph:", server.AreaGraph))
		return nil
	}
	netmask := lsaEnt.Netmask
	network := lsaKey.LSId & netmask
	ent.NbrVertexKey = make([]VertexKey, 0)
	ent.NbrVertexCost = make([]uint16, 0)
	ent.LinkData = make(map[VertexKey]uint32)
	for i := 0; i < len(lsaEnt.AttachedRtr); i++ {
		Rtr := lsaEnt.AttachedRtr[i]
		server.logger.Info(fmt.Sprintln("Attached Router at index:", i, "is:", Rtr))
		var vKey VertexKey
		var cost uint16
		vKey = VertexKey{
			Type:   RouterVertex,
			ID:     Rtr,
			AdvRtr: Rtr,
		}
		cost = 0
		ent.NbrVertexKey = append(ent.NbrVertexKey, vKey)
		ent.NbrVertexCost = append(ent.NbrVertexCost, cost)
		ent.LinkData[vKey] = lsaEnt.Netmask
	}
	ent.AreaId = areaId
	ent.LsaKey = lsaKey
	ent.Visited = false
	//ent.NetMask = lsaEnt.NetMask
	ent.LinkStateId = lsaKey.LSId
	server.AreaGraph[vertexKey] = ent
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No LS Database found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No LS Database found for areaId:", areaId))
		return err
	}
	for _, vKey := range ent.NbrVertexKey {
		_, exist := server.AreaGraph[vKey]
		if exist {
			server.logger.Info(fmt.Sprintln("Entry already exists in SPF Graph for vertexKey:", vertexKey))
			continue
		}
		lsaKey := LsaKey{
			LSType:    RouterLSA,
			LSId:      vKey.ID,
			AdvRouter: vKey.AdvRtr,
		}
		lsaEnt, exist := lsDbEnt.RouterLsaMap[lsaKey]
		if !exist {
			server.logger.Err(fmt.Sprintln("Router LSA with LsaKey:", lsaKey, "not found in areaId:", areaId))
			server.logger.Err(fmt.Sprintln(lsDbEnt))
			server.logger.Err(fmt.Sprintln("======Router LsaMap====", lsDbEnt.RouterLsaMap))
			server.logger.Err(fmt.Sprintln("========Network LsaMap====", lsDbEnt.NetworkLsaMap))
			err := errors.New(fmt.Sprintln("Router LSA with LsaKey:", lsaKey, "not found in areaId:", areaId))
			// continue
			if check == true {
				continue
			}
			return err
		} else {
			flag := false
			for i := 0; i < int(lsaEnt.NumOfLinks); i++ {
				if (lsaEnt.LinkDetails[i].LinkId & netmask) == network {
					if lsaEnt.LinkDetails[i].LinkType == StubLink {
						server.logger.Err(fmt.Sprintln("Have router lsa which still has a stub link in the network, hence ignoring it, lsaKey:", lsaKey, "lsaEnt:", lsaEnt))
						break
					} else if lsaEnt.LinkDetails[i].LinkType == TransitLink {
						server.logger.Err(fmt.Sprintln("Have router lsa which has a transit link in the network, hence processing it, lsaKey:", lsaKey, "lsaEnt:", lsaEnt))
						flag = true
						break
					}
				}
			}
			if flag == false {
				server.logger.Info(fmt.Sprintln("Not able to find the valid router lsa. The Router lsa which we have is the stale one", lsaEnt, lsaKey))
				continue
			}
		}
		err := server.UpdateAreaGraphRouterLsa(lsaEnt, lsaKey, areaId)
		if err != nil {
			if check == true {
				continue
			}
			return err
		}
	}
	return nil
}

func (server *OSPFServer) findNetworkLsa(areaId uint32, LSId uint32) (lsaKey LsaKey, err error) {
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No LS Database found for areaId:", areaId))
		return
	}

	for key, _ := range lsDbEnt.NetworkLsaMap {
		if key.LSId == LSId &&
			key.LSType == NetworkLSA {
			return key, nil
		}
	}

	err = errors.New("Network LSA not found")
	return lsaKey, err
}

func (server *OSPFServer) UpdateAreaGraphRouterLsa(lsaEnt RouterLsa, lsaKey LsaKey, areaId uint32) error {
	server.logger.Info(fmt.Sprintln("1: Using Lsa with key as:", dumpLsaKey(lsaKey), "for SPF calc"))
	vertexKey := VertexKey{
		Type:   RouterVertex,
		ID:     lsaKey.LSId,
		AdvRtr: lsaKey.AdvRouter,
	}
	ent, exist := server.AreaGraph[vertexKey]
	if exist {
		server.logger.Info(fmt.Sprintln("Entry already exists in SPF Graph for vertexKey:", vertexKey))
		server.logger.Info(fmt.Sprintln("SPF Graph:", server.AreaGraph))
		return nil
	}
	if lsaEnt.BitV == true {
		area := config.AreaId(convertUint32ToIPv4(areaId))
		areaConfKey := AreaConfKey{
			AreaId: area,
		}
		aEnt, exist := server.AreaConfMap[areaConfKey]
		if exist && aEnt.TransitCapability == false {
			aEnt.TransitCapability = true
			server.AreaConfMap[areaConfKey] = aEnt
		}
	}
	ent.NbrVertexKey = make([]VertexKey, 0)
	ent.NbrVertexCost = make([]uint16, 0)
	ent.LinkData = make(map[VertexKey]uint32)
	for i := 0; i < int(lsaEnt.NumOfLinks); i++ {
		server.logger.Info(fmt.Sprintln("Link Detail at index", i, "is:", lsaEnt.LinkDetails[i]))
		linkDetail := lsaEnt.LinkDetails[i]
		var vKey VertexKey
		var cost uint16
		var lData uint32
		if linkDetail.LinkType == TransitLink {
			server.logger.Info("===It is TransitLink===")
			vKey = VertexKey{
				Type:   TNetworkVertex,
				ID:     linkDetail.LinkId,
				AdvRtr: 0,
			}
			nLsaKey, err := server.findNetworkLsa(areaId, vKey.ID)
			if err != nil {
				server.logger.Info(fmt.Sprintln("Err:", err, vKey.ID))
				if check == true {
					continue
				}
				return err
				//continue
			}
			vKey.AdvRtr = nLsaKey.AdvRouter
			cost = linkDetail.LinkMetric
			lData = linkDetail.LinkData
			ent.NbrVertexKey = append(ent.NbrVertexKey, vKey)
			ent.NbrVertexCost = append(ent.NbrVertexCost, cost)
			ent.LinkData[vKey] = lData
		} else if linkDetail.LinkType == StubLink {
			server.logger.Info("===It is StubLink===")
			vKey = VertexKey{
				Type:   SNetworkVertex,
				ID:     linkDetail.LinkId,
				AdvRtr: lsaKey.AdvRouter,
			}
			cost = linkDetail.LinkMetric
			lData = linkDetail.LinkData
			sentry, _ := server.AreaStubs[vKey]
			sentry.NbrVertexKey = vertexKey
			sentry.NbrVertexCost = cost
			sentry.LinkData = lData
			sentry.AreaId = areaId
			sentry.LsaKey = lsaKey
			sentry.LinkStateId = lsaKey.LSId
			server.AreaStubs[vKey] = sentry
		} else if linkDetail.LinkType == P2PLink {
			// TODO
		}
	}
	if len(ent.NbrVertexKey) == 0 {
		err := errors.New(fmt.Sprintln("None of the Network LSA are found"))
		return err
	}
	ent.AreaId = areaId
	ent.LsaKey = lsaKey
	ent.Visited = false
	ent.LinkStateId = lsaKey.LSId
	server.AreaGraph[vertexKey] = ent
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No LS Database found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No LS Database found for areaId:", areaId))
		return err
	}
	for _, vKey := range ent.NbrVertexKey {
		_, exist := server.AreaGraph[vKey]
		if exist {
			server.logger.Info(fmt.Sprintln("Entry for Vertex:", vKey, "already exist in Area Graph"))
			continue
		}
		lsaKey := LsaKey{
			LSType:    0,
			LSId:      vKey.ID,
			AdvRouter: vKey.AdvRtr,
		}
		if vKey.Type == TNetworkVertex {
			lsaKey.LSType = NetworkLSA
			lsaEnt, exist := lsDbEnt.NetworkLsaMap[lsaKey]
			if !exist {
				server.logger.Err(fmt.Sprintln("Network LSA with LsaKey:", lsaKey, "not found in LS Database of areaId:", areaId))
				err := errors.New(fmt.Sprintln("Network LSA with LsaKey:", lsaKey, "not found in LS Database of areaId:", areaId))
				if check == true {
					continue
				}
				//continue
				return err
			}
			err := server.UpdateAreaGraphNetworkLsa(lsaEnt, lsaKey, areaId)
			if err != nil {
				if check == true {
					continue
				}
				return err
			}
		} else if vKey.Type == RouterVertex {
			lsaKey.LSType = RouterLSA
			lsaEnt, exist := lsDbEnt.RouterLsaMap[lsaKey]
			if !exist {
				server.logger.Err(fmt.Sprintln("Router LSA with LsaKey:", lsaKey, "not found in LS Database of areaId:", areaId))
				err := errors.New(fmt.Sprintln("Router LSA with LsaKey:", lsaKey, "not found in LS Database of areaId:", areaId))
				//continue
				if check == true {
					continue
				}
				return err
			}
			err := server.UpdateAreaGraphRouterLsa(lsaEnt, lsaKey, areaId)
			if err != nil {
				if check == true {
					continue
				}
				return err
			}
		} else if vKey.Type == SNetworkVertex {
			//TODO
		}
	}
	return nil
}

func (server *OSPFServer) CreateAreaGraph(areaId uint32) (VertexKey, error) {
	var vKey VertexKey
	server.logger.Info(fmt.Sprintln("Create SPF Graph for: areaId:", areaId))
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No LS Database found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No LS Database found for areaId:", areaId))
		return vKey, err
	}

	selfOrigLsaEnt, exist := server.AreaSelfOrigLsa[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No Self Originated LSAs found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No Self Originated LSAs found for areaId:", areaId))
		return vKey, err
	}
	selfRtrLsaKey, err := findSelfOrigRouterLsaKey(selfOrigLsaEnt)
	if err != nil {
		server.logger.Err(fmt.Sprintln("No Self Originated Router LSA Key found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No Self Originated Router LSA Key found for areaId:", areaId))
		return vKey, err
	}
	server.logger.Info(fmt.Sprintln("Self Orginated Router LSA Key:", selfRtrLsaKey))
	lsaEnt, exist := lsDbEnt.RouterLsaMap[selfRtrLsaKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("No Self Originated Router LSA found for areaId:", areaId))
		err := errors.New(fmt.Sprintln("No Self Originated Router LSA found for areaId:", areaId))
		return vKey, err
	}

	err = server.UpdateAreaGraphRouterLsa(lsaEnt, selfRtrLsaKey, areaId)
	if check == true {
		err = nil
	}
	vKey = VertexKey{
		Type:   RouterVertex,
		ID:     selfRtrLsaKey.LSId,
		AdvRtr: selfRtrLsaKey.AdvRouter,
	}
	return vKey, err
}

func (server *OSPFServer) ExecuteDijkstra(vKey VertexKey, areaId uint32) error {
	var treeVSlice []VertexKey = make([]VertexKey, 0)

	treeVSlice = append(treeVSlice, vKey)
	ent, exist := server.SPFTree[vKey]
	if !exist {
		ent.Distance = 0
		ent.NumOfPaths = 1
		ent.Paths = make([]Path, 1)
		var path Path
		path = make(Path, 0)
		ent.Paths[0] = path
		server.SPFTree[vKey] = ent
	}
	for j := 0; j < len(treeVSlice); j++ {
		ent, exist := server.AreaGraph[treeVSlice[j]]
		if !exist {
			server.logger.Info(fmt.Sprintln("No entry found for:", treeVSlice[j]))
			err := errors.New(fmt.Sprintln("No entry found for:", treeVSlice[j]))
			//continue
			return err
		}
		for i := 0; i < len(ent.NbrVertexKey); i++ {
			verKey := ent.NbrVertexKey[i]
			cost := ent.NbrVertexCost[i]
			entry, exist := server.AreaGraph[verKey]
			if !exist {
				server.logger.Info("Something is wrong in SPF Calculation: Entry should exist in Area Graph")
				err := errors.New("Something is wrong in SPF Calculation: Entry should exist in Area Graph")
				return err
			} else {
				if entry.Visited == true {
					continue
				}
			}
			tEnt, exist := server.SPFTree[verKey]
			if !exist {
				tEnt.Paths = make([]Path, 1)
				var path Path
				path = make(Path, 0)
				tEnt.Paths[0] = path
				tEnt.Distance = 0xff00 // LSInfinity
				tEnt.NumOfPaths = 1
			}
			tEntry, exist := server.SPFTree[treeVSlice[j]]
			if !exist {
				server.logger.Err("Something is wrong is SPF Calculation")
				err := errors.New("Something is wrong is SPF Calculation")
				return err
			}
			if tEnt.Distance > tEntry.Distance+cost {
				tEnt.Distance = tEntry.Distance + cost
				for l := 0; l < tEnt.NumOfPaths; l++ {
					tEnt.Paths[l] = nil
				}
				tEnt.Paths = tEnt.Paths[:0]
				tEnt.NumOfPaths = 0
				tEnt.Paths = nil
				tEnt.Paths = make([]Path, tEntry.NumOfPaths)
				for l := 0; l < tEntry.NumOfPaths; l++ {
					var path Path
					path = make(Path, len(tEntry.Paths[l])+1)
					copy(path, tEntry.Paths[l])
					path[len(tEntry.Paths[l])] = treeVSlice[j]
					tEnt.Paths[l] = path
				}
				tEnt.NumOfPaths = tEntry.NumOfPaths
			} else if tEnt.Distance == tEntry.Distance+cost {
				paths := make([]Path, (tEntry.NumOfPaths + tEnt.NumOfPaths))
				for l := 0; l < tEnt.NumOfPaths; l++ {
					var path Path
					path = make(Path, len(tEnt.Paths[l]))
					copy(path, tEnt.Paths[l])
					paths[l] = path
					tEnt.Paths[l] = nil
				}
				tEnt.Paths = tEnt.Paths[:0]
				tEnt.NumOfPaths = 0
				tEnt.Paths = nil
				for l := 0; l < tEntry.NumOfPaths; l++ {
					var path Path
					path = make(Path, len(tEntry.Paths[l])+1)
					copy(path, tEntry.Paths[l])
					path[len(tEntry.Paths[l])] = treeVSlice[j]
					paths[tEnt.NumOfPaths+l] = path
				}
				tEnt.Paths = paths
				tEnt.NumOfPaths = tEntry.NumOfPaths + tEnt.NumOfPaths
			}
			server.SPFTree[verKey] = tEnt
			treeVSlice = append(treeVSlice, verKey)
		}
		ent.Visited = true
		server.AreaGraph[treeVSlice[j]] = ent
	}

	//Handling Stub Networks

	server.logger.Info("Handle Stub Networks")
	for key, entry := range server.AreaStubs {
		//Finding the Vertex(Router) to which this stub is connected to
		vertexKey := entry.NbrVertexKey
		parent, exist := server.SPFTree[vertexKey]
		if !exist {
			continue
		}
		ent, _ := server.SPFTree[key]
		ent.Distance = parent.Distance + entry.NbrVertexCost
		ent.Paths = make([]Path, parent.NumOfPaths)
		for i := 0; i < parent.NumOfPaths; i++ {
			var path Path
			path = make(Path, len(parent.Paths[i])+1)
			copy(path, parent.Paths[i])
			path[len(parent.Paths[i])] = vertexKey
			ent.Paths[i] = path
		}
		ent.NumOfPaths = parent.NumOfPaths
		server.SPFTree[key] = ent
	}
	return nil
}

func dumpVertexKey(key VertexKey) string {
	var Type string
	if key.Type == RouterVertex {
		Type = "Router"
	} else if key.Type == SNetworkVertex {
		Type = "Stub"
	} else if key.Type == TNetworkVertex {
		Type = "Transit"
	}
	ID := convertUint32ToIPv4(key.ID)
	AdvRtr := convertUint32ToIPv4(key.AdvRtr)
	return fmt.Sprintln("Vertex Key[Type:", Type, "ID:", ID, "AdvRtr:", AdvRtr)

}

func dumpLsaKey(key LsaKey) string {
	var Type string
	if key.LSType == RouterLSA {
		Type = "Router LSA"
	} else if key.LSType == NetworkLSA {
		Type = "Network LSA"
	}

	LSId := convertUint32ToIPv4(key.LSId)
	AdvRtr := convertUint32ToIPv4(key.AdvRouter)

	return fmt.Sprintln("LSA Type:", Type, "LSId:", LSId, "AdvRtr:", AdvRtr)
}

func (server *OSPFServer) dumpAreaStubs() {
	server.logger.Info("=======================Dump Area Stubs======================")
	for key, ent := range server.AreaStubs {
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Vertex Keys:", dumpVertexKey(key)))
		server.logger.Info("==================================================")
		LData := convertUint32ToIPv4(ent.LinkData)
		server.logger.Info(fmt.Sprintln("VertexKeys:", dumpVertexKey(ent.NbrVertexKey), "Cost:", ent.NbrVertexCost, "LinkData:", LData))
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Lsa Key:", dumpLsaKey(ent.LsaKey)))
		server.logger.Info(fmt.Sprintln("AreaId:", ent.AreaId))
		server.logger.Info(fmt.Sprintln("LinkStateId:", ent.LinkStateId))
	}
	server.logger.Info("==================================================")

}

func (server *OSPFServer) dumpAreaGraph() {
	server.logger.Info("=======================Dump Area Graph======================")
	for key, ent := range server.AreaGraph {
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Vertex Keys:", dumpVertexKey(key)))
		server.logger.Info("==================================================")
		if len(ent.NbrVertexKey) != 0 {
			server.logger.Info("List of Neighbor Vertices(except stub)")
		} else {
			server.logger.Info("No Neighbor Vertices(except stub)")
		}
		for i := 0; i < len(ent.NbrVertexKey); i++ {
			LData := convertUint32ToIPv4(ent.LinkData[ent.NbrVertexKey[i]])
			server.logger.Info(fmt.Sprintln("VertexKeys:", dumpVertexKey(ent.NbrVertexKey[i]), "Cost:", ent.NbrVertexCost[i], "LinkData:", LData))
		}
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Lsa Key:", dumpLsaKey(ent.LsaKey)))
		server.logger.Info(fmt.Sprintln("AreaId:", ent.AreaId))
		server.logger.Info(fmt.Sprintln("Visited:", ent.Visited))
		server.logger.Info(fmt.Sprintln("LinkStateId:", ent.LinkStateId))
	}
	server.logger.Info("==================================================")
}

func (server *OSPFServer) dumpSPFTree() {
	server.logger.Info("=======================Dump SPF Tree======================")
	for key, ent := range server.SPFTree {
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Vertex Keys:", dumpVertexKey(key)))
		server.logger.Info("==================================================")
		server.logger.Info(fmt.Sprintln("Distance:", ent.Distance))
		server.logger.Info(fmt.Sprintln("NumOfPaths:", ent.NumOfPaths))
		for i := 0; i < ent.NumOfPaths; i++ {
			var paths string
			paths = fmt.Sprintln("Path[", i, "]")
			for j := 0; j < len(ent.Paths[i]); j++ {
				paths = paths + fmt.Sprintln("[", dumpVertexKey(ent.Paths[i][j]), "]")
			}
			server.logger.Info(fmt.Sprintln(paths))
		}

	}
}

func (server *OSPFServer) UpdateRoutingTbl(vKey VertexKey, areaId uint32) {
	areaIdKey := AreaIdKey{
		AreaId: areaId,
	}
	/*
	   lsdbKey := LsdbKey{
	           AreaId: areaId,
	   }
	   lsDbEnt, exist := server.AreaLsdb[lsdbKey]
	   if !exist {
	           server.logger.Err("No Ls DB found..")
	           return
	   }
	*/
	for key, ent := range server.SPFTree {
		if vKey == key {
			server.logger.Info("It's own vertex")
			continue
		}
		switch key.Type {
		case RouterVertex:
			//TODO: If Bit V is set in corresponding RtrLsa set Transit Capability
			// for the given Area
			//rEnt, exist := lsDbEnt.Summary3LsaMap[ent.LsaKey]
			server.UpdateRoutingTblForRouter(areaIdKey, key, ent, vKey)
		case SNetworkVertex:
			server.UpdateRoutingTblForSNetwork(areaIdKey, key, ent, vKey)
		case TNetworkVertex:
			server.UpdateRoutingTblForTNetwork(areaIdKey, key, ent, vKey)
		}
	}
}

func (server *OSPFServer) initRoutingTbl(areaId uint32) {
	server.GlobalRoutingTbl = make(map[RoutingTblEntryKey]GlobalRoutingTblEntry)
}

func (server *OSPFServer) spfCalculation() {
	for {
		msg := <-server.StartCalcSPFCh
		server.logger.Info(fmt.Sprintln("Recevd SPF Calculation Notification for:", msg))
		server.logger.Info(fmt.Sprintln("Area LS Database:", server.AreaLsdb))
		// Create New Routing table
		// Invalidate Old Routing table
		// Backup Old Routing table
		// TODO: Have Per Area Routing Tbl
		server.OldGlobalRoutingTbl = nil
		server.OldGlobalRoutingTbl = make(map[RoutingTblEntryKey]GlobalRoutingTblEntry)
		server.OldGlobalRoutingTbl = server.GlobalRoutingTbl
		server.TempAreaRoutingTbl = nil
		server.TempAreaRoutingTbl = make(map[AreaIdKey]AreaRoutingTbl)
		for key, aEnt := range server.AreaConfMap {
			//flag := false //TODO:Hack
			// Initialize Algorithm's Data Structure

			//server.logger.Info(fmt.Sprintln("===========Area Id : ", key.AreaId, "Area Bdr Status:", server.ospfGlobalConf.isABR, "======================================================="))
			if len(aEnt.IntfListMap) == 0 {
				continue
			}
			aEnt.TransitCapability = false
			areaId := convertAreaOrRouterIdUint32(string(key.AreaId))
			server.AreaGraph = make(map[VertexKey]Vertex)
			server.AreaStubs = make(map[VertexKey]StubVertex)
			server.SPFTree = make(map[VertexKey]TreeVertex)
			areaIdKey := AreaIdKey{
				AreaId: areaId,
			}
			//oldRoutingTbl := server.OldRoutingTbl[areaIdKey]
			//oldRoutingTbl.RoutingTblMap = make(map[RoutingTblEntryKey]RoutingTblEntry)
			//server.OldRoutingTbl[areaIdKey] = oldRoutingTbl
			//server.OldRoutingTbl[areaIdKey] = server.RoutingTbl[areaIdKey]

			tempRoutingTbl := server.TempAreaRoutingTbl[areaIdKey]
			tempRoutingTbl.RoutingTblMap = make(map[RoutingTblEntryKey]RoutingTblEntry)
			server.TempAreaRoutingTbl[areaIdKey] = tempRoutingTbl

			vKey, err := server.CreateAreaGraph(areaId)
			if err != nil {
				server.logger.Err(fmt.Sprintln("Error while creating graph for areaId:", areaId))
				//flag = true
				continue
			}
			//server.logger.Info("=========================Start before Dijkstra=================")
			//server.dumpAreaGraph()
			//server.dumpAreaStubs()
			//server.logger.Info("=========================End before Dijkstra=================")
			//server.printRouterLsa()
			err = server.ExecuteDijkstra(vKey, areaId)
			if err != nil {
				server.logger.Err(fmt.Sprintln("Error while executing Dijkstra for areaId:", areaId))
				//flag = true
				continue
			}
			server.logger.Info("=========================Start after Dijkstra=================")
			server.dumpAreaGraph()
			server.dumpAreaStubs()
			server.dumpSPFTree()
			server.logger.Info("=========================End after Dijkstra=================")
			server.UpdateRoutingTbl(vKey, areaId)
			server.HandleSummaryLsa(areaId)
			server.AreaGraph = nil
			server.AreaStubs = nil
			server.SPFTree = nil
		}
		/*
		   for key, _ := range server.AreaConfMap {
		           areaId := convertAreaOrRouterIdUint32(string(key.AreaId))
		           areaIdKey := AreaIdKey {
		                           AreaId: areaId,
		                   }
		           routingTbl := server.RoutingTbl[areaIdKey]
		           routingTbl.RoutingTblMap = nil
		           server.RoutingTbl[areaIdKey] = routingTbl
		   }
		*/
		server.dumpRoutingTbl() // Per area
		server.TempGlobalRoutingTbl = nil
		server.TempGlobalRoutingTbl = make(map[RoutingTblEntryKey]GlobalRoutingTblEntry)
		/* Summarize and Install/Delete Routes In Routing Table */
		server.InstallRoutingTbl()
		// Copy the Summarize Routing Table in Global Routing Table
		server.GlobalRoutingTbl = nil
		server.GlobalRoutingTbl = make(map[RoutingTblEntryKey]GlobalRoutingTblEntry)
		server.GlobalRoutingTbl = server.TempGlobalRoutingTbl
		server.dumpGlobalRoutingTbl()
		/*
		   for key, _ := range server.AreaConfMap {
		           areaId := convertAreaOrRouterIdUint32(string(key.AreaId))
		           areaIdKey := AreaIdKey {
		                           AreaId: areaId,
		                   }
		           tempRoutingTbl := server.TempRoutingTbl[areaIdKey]
		           routingTbl := make(map[RoutingTblEntryKey]RoutingTblEntry)
		           routingTbl = tempRoutingTbl.RoutingTblMap
		           server.RoutingTbl[areaIdKey] = AreaRoutingTbl {
		                                           RoutingTblMap: routingTbl,
		                                           }
		           tempRoutingTbl.RoutingTblMap = nil
		           server.TempRoutingTbl[areaIdKey] = tempRoutingTbl
		           oldRoutingTbl := server.OldRoutingTbl[areaIdKey]
		           oldRoutingTbl.RoutingTblMap = nil
		           server.OldRoutingTbl[areaIdKey] = oldRoutingTbl
		   }
		*/
		for key, _ := range server.AreaConfMap {
			areaId := convertAreaOrRouterIdUint32(string(key.AreaId))
			areaIdKey := AreaIdKey{
				AreaId: areaId,
			}
			tempAreaRoutingTbl := server.TempAreaRoutingTbl[areaIdKey]
			tempAreaRoutingTbl.RoutingTblMap = nil
			server.TempAreaRoutingTbl[areaIdKey] = tempAreaRoutingTbl
		}
		server.TempAreaRoutingTbl = nil
		server.OldGlobalRoutingTbl = nil
		server.TempGlobalRoutingTbl = nil
		//server.dumpGlobalRoutingTbl()
		if server.ospfGlobalConf.AreaBdrRtrStatus == true {
			server.logger.Info("Examine transit areas, Summary LSA...")
			server.HandleTransitAreaSummaryLsa()
			server.logger.Info("Generate Summary LSA...")
			server.GenerateSummaryLsa()
			server.logger.Info(fmt.Sprintln("========", server.SummaryLsDb, "=========="))
		}
		server.DoneCalcSPFCh <- true
	}
}
