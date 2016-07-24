// ribdv6RouteProcessApis.go
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"l3/rib/ribdCommonDefs"
	"models/objects"
	"net"
	"reflect"
	"ribd"
	"ribdInt"
	"strconv"
	"strings"
	"utils/policy/policyCommonDefs"
)

/*
    This function performs config parameters validation for Route update operation.
	Key validations performed by this fucntion include:
	   - Validate destinationNw. If provided in CIDR notation, convert to ip addr and mask values
*/
func (m RIBDServer) IPv6RouteConfigValidationCheckForUpdate(oldcfg *ribd.IPv6Route, cfg *ribd.IPv6Route, attrset []bool) (err error) {
	logger.Info(fmt.Sprintln("IPv6RouteConfigValidationCheckForUpdate"))
	isCidr := strings.Contains(cfg.DestinationNw, "/")
	if isCidr {
		/*
		   the given address is in CIDR format
		*/
		ip, ipNet, err := net.ParseCIDR(cfg.DestinationNw)
		if err != nil {
			logger.Err(fmt.Sprintln("Invalid Destination IP address"))
			return errors.New("Invalid Desitnation IP address")
		}
		_, err = getNetworkPrefixFromCIDR(cfg.DestinationNw)
		if err != nil {
			return errors.New("Invalid destination ip/network Mask")
		}
		cfg.DestinationNw = ip.String()
		oldcfg.DestinationNw = ip.String()
		ipMask := make(net.IP, 16)
		copy(ipMask, ipNet.Mask)
		ipMaskStr := net.IP(ipMask).String()
		cfg.NetworkMask = ipMaskStr
		oldcfg.NetworkMask = ipMaskStr
	}
	_, err = validateNetworkPrefix(cfg.DestinationNw, cfg.NetworkMask)
	if err != nil {
		logger.Info(fmt.Sprintln(" getNetowrkPrefixFromStrings returned err ", err))
		return errors.New("Invalid destination ip address")
	}
	/*
		    Default operation for update function is to update route Info. The following
			logic deals with updating route attributes
	*/
	if attrset != nil {
		logger.Debug("attr set not nil, set individual attributes")
		objTyp := reflect.TypeOf(*cfg)
		for i := 0; i < objTyp.NumField(); i++ {
			objName := objTyp.Field(i).Name
			if attrset[i] {
				logger.Debug(fmt.Sprintf("Processv6RouteUpdateConfig (server validation): changed ", objName))
				if objName == "Protocol" {
					/*
					   Updating route protocol type is not allowed
					*/
					logger.Err("Cannot update Protocol value of a route")
					return errors.New("Cannot set Protocol field")
				}
				if objName == "NextHop" {
					/*
					   Next hop info is being updated
					*/
					if len(cfg.NextHop) == 0 {
						/*
						   Expects non-zero nexthop info
						*/
						logger.Err("Must specify next hop")
						return errors.New("Next hop ip not specified")
					}
					/*
					   Check if next hop IP is valid
					*/
					for i := 0; i < len(cfg.NextHop); i++ {
						_, err = getIP(cfg.NextHop[i].NextHopIp)
						if err != nil {
							logger.Err(fmt.Sprintln("nextHopIpAddr invalid"))
							return errors.New("Invalid next hop ip address")
						}
						/*
						   Check if next hop intf is valid L3 interface
						*/
						if cfg.NextHop[i].NextHopIntRef != "" {
							logger.Debug(fmt.Sprintln("IntRef before : ", cfg.NextHop[i].NextHopIntRef))
							cfg.NextHop[i].NextHopIntRef, err = m.ConvertIntfStrToIfIndexStr(cfg.NextHop[i].NextHopIntRef)
							if err != nil {
								logger.Err(fmt.Sprintln("Invalid NextHop IntRef ", cfg.NextHop[i].NextHopIntRef))
								return errors.New("Invalid Nexthop Intref")
							}
							logger.Debug(fmt.Sprintln("IntRef after : ", cfg.NextHop[0].NextHopIntRef))
						} else {
							if len(oldcfg.NextHop) == 0 || len(oldcfg.NextHop) < i {
								logger.Err("Number of nextHops for old cfg < new cfg")
								return errors.New("number of nexthops not correct for update replace operation")
							}
							logger.Debug(fmt.Sprintln("IntRef not provided, take the old value", oldcfg.NextHop[i].NextHopIntRef))
							cfg.NextHop[i].NextHopIntRef, err = m.ConvertIntfStrToIfIndexStr(oldcfg.NextHop[i].NextHopIntRef)
							if err != nil {
								logger.Err(fmt.Sprintln("Invalid NextHop IntRef ", oldcfg.NextHop[i].NextHopIntRef))
								return errors.New("Invalid Nexthop Intref")
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func (m RIBDServer) IPv6RouteConfigValidationCheckForPatchUpdate(oldcfg *ribd.IPv6Route, cfg *ribd.IPv6Route, op []*ribd.PatchOpInfo) (err error) {
	logger.Info(fmt.Sprintln("IPv6RouteConfigValidationCheckForPatchUpdate"))
	isCidr := strings.Contains(cfg.DestinationNw, "/")
	if isCidr {
		logger.Debug("cidr address")
		/*
		   the given address is in CIDR format
		*/
		ip, ipNet, err := net.ParseCIDR(cfg.DestinationNw)
		if err != nil {
			logger.Err(fmt.Sprintln("Invalid Destination IP address"))
			return errors.New("Invalid Desitnation IP address")
		}
		_, err = getNetworkPrefixFromCIDR(cfg.DestinationNw)
		if err != nil {
			return errors.New("Invalid destination ip/network Mask")
		}
		logger.Debug(fmt.Sprintln("At the beginning oldcfg.destinationnw:", oldcfg.DestinationNw, " cfg.DesinationNw:", cfg.DestinationNw))
		cfg.DestinationNw = ip.String()
		oldcfg.DestinationNw = ip.String()
		ipMask := make(net.IP, 16)
		copy(ipMask, ipNet.Mask)
		ipMaskStr := net.IP(ipMask).String()
		cfg.NetworkMask = ipMaskStr
		oldcfg.NetworkMask = ipMaskStr
		logger.Debug(fmt.Sprintln("After conversion oldcfg.destinationnw:", oldcfg.DestinationNw, " cfg.DesinationNw:", cfg.DestinationNw))
	}
	_, err = validateNetworkPrefix(cfg.DestinationNw, cfg.NetworkMask)
	if err != nil {
		logger.Info(fmt.Sprintln(" getNetowrkPrefixFromStrings returned err ", err))
		return errors.New("Invalid destination ip address")
	}
	for idx := 0; idx < len(op); idx++ {
		logger.Debug(fmt.Sprintln("patch update"))
		switch op[idx].Path {
		case "NextHop":
			logger.Debug("Patch update for next hop")
			if len(op[idx].Value) == 0 {
				/*
					If route update is trying to add next hop, non zero nextHop info is expected
				*/
				logger.Err("Must specify next hop")
				return errors.New("Next hop ip not specified")
			}
			logger.Debug(fmt.Sprintln("value = ", op[idx].Value))
			valueObjArr := []ribd.NextHopInfo{}
			err = json.Unmarshal([]byte(op[idx].Value), &valueObjArr)
			if err != nil {
				logger.Debug(fmt.Sprintln("error unmarshaling value:", err))
				return errors.New(fmt.Sprintln("error unmarshaling value:", err))
			}
			logger.Debug(fmt.Sprintln("Number of nextHops:", len(valueObjArr)))
			for _, val := range valueObjArr {
				/*
				   Check if the next hop ip valid
				*/
				logger.Debug(fmt.Sprintln("nextHop info: ip - ", val.NextHopIp, " intf: ", val.NextHopIntRef, " wt:", val.Weight))
				_, err = getIP(val.NextHopIp)
				if err != nil {
					logger.Err(fmt.Sprintln("nextHopIpAddr invalid"))
					return errors.New("Invalid next hop ip address")
				}

				switch op[idx].Op {
				case "add":
					/*
					   Check if the next hop ref is valid L3 interface for add operation
					*/
					logger.Debug(fmt.Sprintln("IntRef before : ", val.NextHopIntRef))
					if val.NextHopIntRef == "" {
						logger.Info(fmt.Sprintln("NextHopIntRef not set"))
						nhIntf, err := RouteServiceHandler.GetRouteReachabilityInfo(val.NextHopIp)
						if err != nil {
							logger.Err(fmt.Sprintln("next hop ip ", val.NextHopIp, " not reachable"))
							return errors.New(fmt.Sprintln("next hop ip ", val.NextHopIp, " not reachable"))
						}
						val.NextHopIntRef = strconv.Itoa(int(nhIntf.NextHopIfIndex))
					} else {
						val.NextHopIntRef, err = m.ConvertIntfStrToIfIndexStr(val.NextHopIntRef)
						if err != nil {
							logger.Err(fmt.Sprintln("Invalid NextHop IntRef ", val.NextHopIntRef))
							return err
						}
					}
					logger.Debug(fmt.Sprintln("IntRef after : ", val.NextHopIntRef))
				case "remove":
					logger.Debug(fmt.Sprintln("remove op"))
				default:
					logger.Err(fmt.Sprintln("operation ", op[idx].Op, " not supported"))
					return errors.New(fmt.Sprintln("operation ", op[idx].Op, " not supported"))
				}
			}
		default:
			logger.Err(fmt.Sprintln("Patch update for attribute:", op[idx].Path, " not supported"))
			return errors.New("Invalid attribute for patch update")
		}
	}

	return nil
}

/*
    This function performs config parameters validation for op = "add" and "del" values.
	Key validations performed by this fucntion include:
	   - if the Protocol specified is valid (STATIC/CONNECTED/EBGP/OSPF)
	   - Validate destinationNw. If provided in CIDR notation, convert to ip addr and mask values
	   - In case of op == "del", check if the route is present in the DB
	   - for each of the nextHop info, check:
	       - if the next hop ip is valid
		   - if the nexthopIntf is valid L3 intf and if so, convert to string value
*/
func (m RIBDServer) IPv6RouteConfigValidationCheck(cfg *ribd.IPv6Route, op string) (err error) {
	logger.Debug(fmt.Sprintln("IPv6RouteConfigValidationCheck"))
	isCidr := strings.Contains(cfg.DestinationNw, "/")
	if isCidr {
		/*
		   the given address is in CIDR format
		*/
		ip, ipNet, err := net.ParseCIDR(cfg.DestinationNw)
		if err != nil {
			logger.Err(fmt.Sprintln("Invalid Destination IP address"))
			return errors.New("Invalid Desitnation IP address")
		}
		_, err = getNetworkPrefixFromCIDR(cfg.DestinationNw)
		if err != nil {
			return errors.New("Invalid destination ip/network Mask")
		}
		/*
		   Convert the CIDR format address to IP and mask strings
		*/
		cfg.DestinationNw = ip.String()
		ipMask := make(net.IP, 16)
		copy(ipMask, ipNet.Mask)
		ipMaskStr := net.IP(ipMask).String()
		cfg.NetworkMask = ipMaskStr
		/*
			In case where user provides CIDR address, the DB cannot verify if the route is present, so check here
		*/
		if m.DbHdl != nil {
			var dbObjCfg objects.IPv6Route
			dbObjCfg.DestinationNw = cfg.DestinationNw
			dbObjCfg.NetworkMask = cfg.NetworkMask
			key := "IPv4Route#" + cfg.DestinationNw + "#" + cfg.NetworkMask
			_, err := m.DbHdl.GetObjectFromDb(dbObjCfg, key)
			if err == nil {
				logger.Err("Duplicate entry")
				return errors.New("Duplicate entry")
			}
		}
	}
	_, err = validateNetworkPrefix(cfg.DestinationNw, cfg.NetworkMask)
	if err != nil {
		logger.Info(fmt.Sprintln(" getNetowrkPrefixFromStrings returned err ", err))
		return err
	}
	/*
	   op is to add new route
	*/
	if op == "add" {
		/*
		   check if route protocol type is valid
		*/
		_, ok := RouteProtocolTypeMapDB[cfg.Protocol]
		if !ok {
			logger.Err(fmt.Sprintln("route type ", cfg.Protocol, " invalid"))
			err = errors.New("Invalid route protocol type")
			return err
		}
		logger.Debug(fmt.Sprintln("Number of nexthops = ", len(cfg.NextHop)))
		if len(cfg.NextHop) == 0 {
			/*
				Expects non-zero nexthop info
			*/
			logger.Err("Must specify next hop")
			return errors.New("Next hop ip not specified")
		}
		for i := 0; i < len(cfg.NextHop); i++ {
			/*
			   Check if the NextHop IP valid
			*/
			_, err = getIP(cfg.NextHop[i].NextHopIp)
			if err != nil {
				logger.Err(fmt.Sprintln("nextHopIpAddr invalid"))
				return errors.New("Invalid next hop ip address")
			}
			logger.Debug(fmt.Sprintln("IntRef before : ", cfg.NextHop[i].NextHopIntRef))
			/*
			   Validate if nextHopIntRef is a valid L3 interface
			*/
			if cfg.NextHop[i].NextHopIntRef == "" {
				logger.Info(fmt.Sprintln("NextHopIntRef not set"))
				nhIntf, err := RouteServiceHandler.GetRouteReachabilityInfo(cfg.NextHop[i].NextHopIp)
				if err != nil {
					logger.Err(fmt.Sprintln("next hop ip ", cfg.NextHop[i].NextHopIp, " not reachable"))
					return errors.New(fmt.Sprintln("next hop ip ", cfg.NextHop[i].NextHopIp, " not reachable"))
				}
				cfg.NextHop[i].NextHopIntRef = strconv.Itoa(int(nhIntf.NextHopIfIndex))
			} else {
				cfg.NextHop[i].NextHopIntRef, err = m.ConvertIntfStrToIfIndexStr(cfg.NextHop[i].NextHopIntRef)
				if err != nil {
					logger.Err(fmt.Sprintln("Invalid NextHop IntRef ", cfg.NextHop[i].NextHopIntRef))
					return err
				}
			}
			logger.Debug(fmt.Sprintln("IntRef after : ", cfg.NextHop[i].NextHopIntRef))
		}
	}
	return nil
}
func (m RIBDServer) Getv6Route(destNetIp string) (route *ribdInt.IPv6RouteState, err error) {
	var returnRoute ribdInt.IPv6RouteState
	route = &returnRoute
	/*
	   the given address is in CIDR format
	*/
	destNet, err := getNetworkPrefixFromCIDR(destNetIp)
	if err != nil {
		return route, errors.New("Invalid destination ip/network Mask")
	}
	routeInfoRecordListItem := RouteInfoMap.Get(destNet)
	if routeInfoRecordListItem == nil {
		logger.Debug("No such route")
		err = errors.New("Route does not exist")
		return route, err
	}
	routeInfoRecordList := routeInfoRecordListItem.(RouteInfoRecordList) //RouteInfoMap.Get(destNet).(RouteInfoRecordList)
	if routeInfoRecordList.selectedRouteProtocol == "INVALID" {
		logger.Debug("No selected route for this network")
		err = errors.New("No selected route for this network")
		return route, err
	}
	routeInfoList := routeInfoRecordList.routeInfoProtocolMap[routeInfoRecordList.selectedRouteProtocol]
	nextHopInfo := make([]ribdInt.RouteNextHopInfo, len(routeInfoList))
	route.NextHopList = make([]*ribdInt.RouteNextHopInfo, 0)
	i := 0
	for _, nh := range routeInfoList {
		routeInfoRecord := nh
		nextHopInfo[i].NextHopIp = routeInfoRecord.nextHopIp.String()
		nextHopInfo[i].NextHopIntRef = strconv.Itoa(int(routeInfoRecord.nextHopIfIndex))
		intfEntry, ok := IntfIdNameMap[int32(routeInfoRecord.nextHopIfIndex)]
		if ok {
			logger.Debug(fmt.Sprintln("Map found for ifndex : ", routeInfoRecord.nextHopIfIndex, "Name = ", intfEntry.name))
			nextHopInfo[i].NextHopIntRef = intfEntry.name
		}
		logger.Debug(fmt.Sprintln("IntfRef = ", nextHopInfo[i].NextHopIntRef))
		nextHopInfo[i].Weight = int32(routeInfoRecord.weight)
		route.NextHopList = append(route.NextHopList, &nextHopInfo[i])
		i++

	}
	routeInfoRecord := routeInfoList[0]
	route.DestinationNw = routeInfoRecord.networkAddr
	route.Protocol = routeInfoRecordList.selectedRouteProtocol
	route.RouteCreatedTime = routeInfoRecord.routeCreatedTime
	route.RouteUpdatedTime = routeInfoRecord.routeUpdatedTime
	return route, err
}

func (m RIBDServer) ProcessV6RouteCreateConfig(cfg *ribd.IPv6Route) (val bool, err error) {
	logger.Debug(fmt.Sprintln("ProcessV6RouteCreate: Received create route request for ip: ", cfg.DestinationNw, " mask ", cfg.NetworkMask, " number of next hops: ", len(cfg.NextHop)))
	newCfg := ribd.IPv6Route{
		DestinationNw: cfg.DestinationNw,
		NetworkMask:   cfg.NetworkMask,
		Protocol:      cfg.Protocol,
		Cost:          cfg.Cost,
		NullRoute:     cfg.NullRoute,
	}
	for i := 0; i < len(cfg.NextHop); i++ {
		logger.Debug(fmt.Sprintln("nexthop info: ip: ", cfg.NextHop[i].NextHopIp, " intref: ", cfg.NextHop[i].NextHopIntRef))
		nh := ribd.NextHopInfo{
			NextHopIp:     cfg.NextHop[i].NextHopIp,
			NextHopIntRef: cfg.NextHop[i].NextHopIntRef,
			Weight:        cfg.NextHop[i].Weight,
		}
		newCfg.NextHop = make([]*ribd.NextHopInfo, 0)
		newCfg.NextHop = append(newCfg.NextHop, &nh)
	}

	policyRoute := BuildPolicyRouteFromribdIPv6Route(&newCfg)
	params := BuildRouteParamsFromribdIPv6Route(&newCfg, FIBAndRIB, Invalid, len(destNetSlice))

	logger.Debug(fmt.Sprintln("createType = ", params.createType, "deleteType = ", params.deleteType))
	PolicyEngineFilter(policyRoute, policyCommonDefs.PolicyPath_Import, params)

	return true, err
}

func (m RIBDServer) ProcessV6RouteDeleteConfig(cfg *ribd.IPv6Route) (val bool, err error) {
	logger.Debug(fmt.Sprintln("ProcessRoutev6DeleteConfig:Received Route Delete request for ", cfg.DestinationNw, ":", cfg.NetworkMask, "number of nextHops:", len(cfg.NextHop), "Protocol ", cfg.Protocol))
	if !RouteServiceHandler.AcceptConfig {
		logger.Debug("Not ready to accept config")
		//return 0,err
	}
	for i := 0; i < len(cfg.NextHop); i++ {
		logger.Debug(fmt.Sprintln("nexthop info: ip: ", cfg.NextHop[i].NextHopIp, " intref: ", cfg.NextHop[i].NextHopIntRef))
		_, err = deleteIPRoute(cfg.DestinationNw, cfg.NetworkMask, cfg.Protocol, cfg.NextHop[i].NextHopIp, FIBAndRIB, ribdCommonDefs.RoutePolicyStateChangetoInValid)
	}
	return true, err
}

func (m RIBDServer) Processv6RoutePatchUpdateConfig(origconfig *ribd.IPv6Route, newconfig *ribd.IPv6Route, op []*ribd.PatchOpInfo) (ret bool, err error) {
	logger.Debug(fmt.Sprintln("Processv6RoutePatchUpdateConfig:Received update route request with number of patch ops: ", len(op)))
	if !RouteServiceHandler.AcceptConfig {
		logger.Debug("Not ready to accept config")
		//return err
	}
	destNet, err := getNetowrkPrefixFromStrings(origconfig.DestinationNw, origconfig.NetworkMask)
	if err != nil {
		logger.Debug(fmt.Sprintln(" getNetowrkPrefixFromStrings returned err ", err))
		return ret, err
	}
	ok := RouteInfoMap.Match(destNet)
	if !ok {
		err = errors.New("No route found")
		return ret, err
	}
	for idx := 0; idx < len(op); idx++ {
		switch op[idx].Path {
		case "NextHop":
			logger.Debug("Patch update for next hop")
			/*newconfig should only have the next hops that have to be added or deleted*/
			newconfig.NextHop = make([]*ribd.NextHopInfo, 0)
			logger.Debug(fmt.Sprintln("value = ", op[idx].Value))
			valueObjArr := []ribd.NextHopInfo{}
			err = json.Unmarshal([]byte(op[idx].Value), &valueObjArr)
			if err != nil {
				logger.Debug(fmt.Sprintln("error unmarshaling value:", err))
				return ret, errors.New(fmt.Sprintln("error unmarshaling value:", err))
			}
			logger.Debug(fmt.Sprintln("Number of nextHops:", len(valueObjArr)))
			for _, val := range valueObjArr {
				logger.Debug(fmt.Sprintln("nextHop info: ip - ", val.NextHopIp, " intf: ", val.NextHopIntRef, " wt:", val.Weight))
				//wt,_ := strconv.Atoi((op[idx].Value[j]["Weight"]))
				logger.Debug(fmt.Sprintln("IntRef before : ", val.NextHopIntRef))
				if val.NextHopIntRef == "" {
					logger.Info(fmt.Sprintln("NextHopIntRef not set"))
					nhIntf, err := RouteServiceHandler.GetRouteReachabilityInfo(val.NextHopIp)
					if err != nil {
						logger.Err(fmt.Sprintln("next hop ip ", val.NextHopIp, " not reachable"))
						return ret, errors.New(fmt.Sprintln("next hop ip ", val.NextHopIp, " not reachable"))
					}
					val.NextHopIntRef = strconv.Itoa(int(nhIntf.NextHopIfIndex))
				} else {
					val.NextHopIntRef, err = m.ConvertIntfStrToIfIndexStr(val.NextHopIntRef)
					if err != nil {
						logger.Err(fmt.Sprintln("Invalid NextHop IntRef ", val.NextHopIntRef))
						return ret, err
					}
				}
				logger.Debug(fmt.Sprintln("IntRef after : ", val.NextHopIntRef))
				nh := ribd.NextHopInfo{
					NextHopIp:     val.NextHopIp,
					NextHopIntRef: val.NextHopIntRef,
					Weight:        val.Weight,
				}
				newconfig.NextHop = append(newconfig.NextHop, &nh)
			}
			switch op[idx].Op {
			case "add":
				m.ProcessV6RouteCreateConfig(newconfig)
			case "remove":
				m.ProcessV6RouteDeleteConfig(newconfig)
			default:
				logger.Err(fmt.Sprintln("Operation ", op[idx].Op, " not supported"))
			}
		default:
			logger.Err(fmt.Sprintln("Patch update for attribute:", op[idx].Path, " not supported"))
		}
	}
	return ret, err
}

func (m RIBDServer) Processv6RouteUpdateConfig(origconfig *ribd.IPv6Route, newconfig *ribd.IPv6Route, attrset []bool) (val bool, err error) {
	logger.Debug(fmt.Sprintln("Processv6RouteUpdateConfig:Received update route request origconfig.DestinationNw:", origconfig.DestinationNw, " newconfig.DestinationNw:", newconfig.DestinationNw))
	if !RouteServiceHandler.AcceptConfig {
		logger.Debug("Not ready to accept config")
		//return err
	}
	destNet, err := getNetowrkPrefixFromStrings(origconfig.DestinationNw, origconfig.NetworkMask)
	if err != nil {
		logger.Debug(fmt.Sprintln(" getNetowrkPrefixFromStrings returned err ", err))
		return val, err
	}
	ok := RouteInfoMap.Match(destNet)
	if !ok {
		err = errors.New(fmt.Sprintln("No route found for ip ", destNet))
		return val, err
	}
	routeInfoRecordListItem := RouteInfoMap.Get(destNet)
	if routeInfoRecordListItem == nil {
		logger.Debug(fmt.Sprintln("No route for destination network", destNet))
		return val, err
	}
	routeInfoRecordList := routeInfoRecordListItem.(RouteInfoRecordList)
	callUpdate := true
	if attrset != nil {
		found, routeInfoRecord, index := findRouteWithNextHop(routeInfoRecordList.routeInfoProtocolMap[origconfig.Protocol], origconfig.NextHop[0].NextHopIp)
		if !found || index == -1 {
			logger.Debug("Invalid nextHopIP")
			return val, err
		}
		objTyp := reflect.TypeOf(*origconfig)
		for i := 0; i < objTyp.NumField(); i++ {
			objName := objTyp.Field(i).Name
			if attrset[i] {
				logger.Debug(fmt.Sprintf("Processv6RouteUpdateConfig (server): changed ", objName))
				if objName == "NextHop" {
					if len(newconfig.NextHop) == 0 {
						logger.Err("Must specify next hop")
						return val, err
					} else {
						nextHopIpAddr, err := getIP(newconfig.NextHop[0].NextHopIp)
						if err != nil {
							logger.Debug("nextHopIpAddr invalid")
							return val, errors.New("Invalid next hop")
						}
						logger.Debug(fmt.Sprintln("Update the next hop info old ip: ", origconfig.NextHop[0].NextHopIp, " new value: ", newconfig.NextHop[0].NextHopIp, " weight : ", newconfig.NextHop[0].Weight))
						routeInfoRecord.nextHopIp = nextHopIpAddr
						routeInfoRecord.weight = ribd.Int(newconfig.NextHop[0].Weight)
						if newconfig.NextHop[0].NextHopIntRef != "" {
							nextHopIntRef, _ := strconv.Atoi(newconfig.NextHop[0].NextHopIntRef)
							routeInfoRecord.nextHopIfIndex = ribd.Int(nextHopIntRef)
						}
					}
				}
				if objName == "Cost" {
					routeInfoRecord.metric = ribd.Int(newconfig.Cost)
				}
				/*				if objName == "OutgoingInterface" {
								nextHopIfIndex, _ := strconv.Atoi(newconfig.OutgoingInterface)
								routeInfoRecord.nextHopIfIndex = ribd.Int(nextHopIfIndex)
								callUpdate = false
							}*/
			}
		}
		routeInfoRecordList.routeInfoProtocolMap[origconfig.Protocol][index] = routeInfoRecord
		RouteInfoMap.Set(destNet, routeInfoRecordList)
		logger.Debug("Adding to DBRouteCh from processRouteUpdateConfig")
		RouteServiceHandler.DBRouteCh <- RIBdServerConfig{
			OrigConfigObject: RouteDBInfo{routeInfoRecord, routeInfoRecordList},
			Op:               "add",
		}
		//RouteServiceHandler.WriteIPv4RouteStateEntryToDB(RouteDBInfo{routeInfoRecord, routeInfoRecordList})
		if callUpdate == false {
			return val, err
		}
	}
	updateBestRoute(destNet, routeInfoRecordList)
	return val, err
}
