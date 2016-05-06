package ribdCommonDefs

import (
	"ribdInt"
	"utils/commonDefs"
)

const (
	CONNECTED                               = 0
	STATIC                                  = 1
	OSPF                                    = 89
	EBGP                                    = 8
	IBGP                                    = 9
	BGP                                     = 17
	PUB_SOCKET_ADDR                         = "ipc:///tmp/ribd.ipc"
	PUB_SOCKET_BGPD_ADDR                    = "ipc:///tmp/ribd_bgpd.ipc"
	PUB_SOCKET_OSPFD_ADDR                   = "ipc:///tmp/ribd_ospfd.ipc"
	PUB_SOCKET_BFDD_ADDR                    = "ipc:///tmp/ribd_bfdd.ipc"
	PUB_SOCKET_POLICY_ADDR                  = "ipc:///tmp/ribd_policyd.ipc"
	NOTIFY_ROUTE_CREATED                    = 1
	NOTIFY_ROUTE_DELETED                    = 2
	NOTIFY_ROUTE_INVALIDATED                = 3
	NOTIFY_ROUTE_REACHABILITY_STATUS_UPDATE = 4
	NOTIFY_POLICY_CONDITION_CREATED         = 5
	NOTIFY_POLICY_CONDITION_DELETED         = 6
	NOTIFY_POLICY_CONDITION_UPDATED         = 7
	NOTIFY_POLICY_STMT_CREATED              = 8
	NOTIFY_POLICY_STMT_DELETED              = 9
	NOTIFY_POLICY_STMT_UPDATED              = 10
	NOTIFY_POLICY_DEFINITION_CREATED        = 11
	NOTIFY_POLICY_DEFINITION_DELETED        = 12
	NOTIFY_POLICY_DEFINITION_UPDATED        = 13
	DEFAULT_NOTIFICATION_SIZE               = 128
	RoutePolicyStateChangetoValid           = 1
	RoutePolicyStateChangetoInValid         = 2
	RoutePolicyStateChangeNoChange          = 3
)

type RibdNotifyMsg struct {
	MsgType uint16
	MsgBuf  []byte
}

type RoutelistInfo struct {
	RouteInfo ribdInt.Routes
}
type RouteReachabilityStatusMsgInfo struct {
	Network     string
	IsReachable bool
	NextHopIntf ribdInt.NextHopInfo
}
func GetNextHopIfTypeStr(nextHopIfType ribdInt.Int) (nextHopIfTypeStr string, err error) {
	nextHopIfTypeStr = ""
	switch nextHopIfType {
	case commonDefs.IfTypePort:
		nextHopIfTypeStr = "PHY"
		break
	case commonDefs.IfTypeVlan:
		nextHopIfTypeStr = "VLAN"
		break
	case commonDefs.IfTypeNull:
		nextHopIfTypeStr = "NULL"
		break
	case commonDefs.IfTypeLoopback:
		nextHopIfTypeStr = "Loopback"
		break
	}
	return nextHopIfTypeStr, err
}
