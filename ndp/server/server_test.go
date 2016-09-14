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
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	_ "github.com/google/gopacket/pcap"
	"infra/sysd/sysdCommonDefs"
	"l3/ndp/config"
	"l3/ndp/debug"
	"log/syslog"
	"testing"
	asicdmock "utils/asicdClient/mock"
	"utils/logging"
)

var nsServerBaseTestPkt = []byte{
	0x00, 0x1f, 0x16, 0x25, 0x34, 0x31, 0x00, 0x1f, 0x16, 0x25, 0x33, 0xce, 0x86, 0xdd, 0x60, 0x00,
	0x00, 0x00, 0x00, 0x20, 0x3a, 0xff, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x1f,
	0x16, 0xff, 0xfe, 0x25, 0x33, 0xce, 0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0xf1, 0x01, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x87, 0x00, 0xa6, 0x86, 0x00, 0x00, 0x00, 0x00, 0x20, 0x01,
	0x0d, 0xb8, 0x00, 0x00, 0xf1, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x01,
	0x00, 0x1f, 0x16, 0x25, 0x33, 0xce,
}
var raServerBaseTestPkt = []byte{
	0x33, 0x33, 0x00, 0x00, 0x00, 0x01, 0x88, 0x1d, 0xfc, 0xcf, 0x15, 0xfc, 0x86, 0xdd, 0x60, 0x00,
	0x00, 0x00, 0x00, 0x20, 0x3a, 0xff, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x8a, 0x1d,
	0xfc, 0xff, 0xfe, 0xcf, 0x15, 0xfc, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x86, 0x00, 0xf2, 0x66, 0x40, 0x00, 0x07, 0x08, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x88, 0x1d, 0xfc, 0xcf, 0x15, 0xfc, 0x05, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x05, 0xdc,
}

// eth1_icmpv6.pcap
var naServerBaseTestPkt = []byte{
	0x00, 0x1f, 0x16, 0x25, 0x3e, 0x71, 0xf6, 0x6d, 0xe4, 0x22, 0x75, 0x9e, 0x86, 0xdd, 0x60, 0x00,
	0x00, 0x00, 0x00, 0x20, 0x3a, 0xff, 0x21, 0x49, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x61, 0x01, 0x23, 0x00, 0x01, 0x21, 0x49, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x61, 0x01, 0x23, 0x00, 0x02, 0x88, 0x00, 0xdd, 0x08, 0xe0, 0x00, 0x00, 0x00, 0x21, 0x49,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x61, 0x01, 0x23, 0x00, 0x01, 0x02, 0x01,
	0xf6, 0x6d, 0xe4, 0x22, 0x75, 0x9e,
}

const (
	TEST_NBR_ENTRIES     = 5
	testIfIndex          = 100
	testIntfRef          = "lo"
	testSwitchMac        = "c8:1f:66:ea:ae:fc"
	testMyGSIp           = "2192::168:1:1/64"
	testMyLinkScopeIP    = "fe80::77:9cf8:fcff:fe4a:1615/16"
	testMyAbsGSIP        = "2192::168:1:1"
	testMyAbsLinkScopeIP = "fe80::77:9cf8:fcff:fe4a:1615"
	testSrcMac           = "88:1d:fc:cf:15:fc"

	testServerNSSrcMac = "00:1f:16:25:33:ce"
	testServerNSDstMac = "00:1f:16:25:34:31"
	testServerNSSrcIp  = "fe80::21f:16ff:fe25:33ce"
	testServerNSDstIp  = "2001:db8:0:f101::1"

	testReachableTimerValue = 30000
	estReTransmitTimerValue = 1000
)

var testNdpServer *NDPServer
var testIpv6GSNotifyObj *config.IPIntfNotification
var testIpv6LSNotifyObj *config.IPIntfNotification
var testServerInitdone chan bool
var testServerQuit chan bool

var testPorts []config.PortInfo

func NDPTestNewLogger(name string, tag string, listenToConfig bool) (*logging.Writer, error) {
	var err error
	srLogger := new(logging.Writer)
	srLogger.MyComponentName = name

	srLogger.SysLogger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, tag)
	if err != nil {
		fmt.Println("Failed to initialize syslog - ", err)
		return srLogger, err
	}

	srLogger.GlobalLogging = true
	srLogger.MyLogLevel = sysdCommonDefs.INFO
	return srLogger, err
}

func initServerBasic() {
	t := &testing.T{}
	logger, err := NDPTestNewLogger("ndpd", "NDPTEST", true)
	if err != nil {
		t.Error("creating logger failed")
	}
	debug.NDPSetLogger(logger)
}

func initPhysicalPorts() {
	port := config.PortInfo{
		IntfRef:   "lo",
		IfIndex:   testIfIndex,
		Name:      "Loopback0",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
	port = config.PortInfo{
		IntfRef:   "lo1",
		IfIndex:   96,
		Name:      "Loopback1",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
	port = config.PortInfo{
		IntfRef:   "lo2",
		IfIndex:   97,
		Name:      "Loopback2",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
	port = config.PortInfo{
		IntfRef:   "lo3",
		IfIndex:   98,
		Name:      "Loopback3",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
	port = config.PortInfo{
		IntfRef:   "lo4",
		IfIndex:   99,
		Name:      "Loopback4",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
	port = config.PortInfo{
		IntfRef:   "lo5",
		IfIndex:   95,
		Name:      "Loopback5",
		OperState: "UP",
		MacAddr:   "aa:bb:cc:dd:ee:ff",
	}
	testNdpServer.PhyPort[port.IfIndex] = port
}

func InitNDPTestServer() {
	initServerBasic()
	testServerInitdone = make(chan bool)
	testServerQuit = make(chan bool)
	testNdpServer = NDPNewServer(&asicdmock.MockAsicdClientMgr{}, nil)
	testNdpServer.NDPStartServer()
	initPhysicalPorts()
	testIpv6GSNotifyObj = &config.IPIntfNotification{
		IfIndex: testIfIndex,
		IpAddr:  testMyGSIp,
	}

	testIpv6LSNotifyObj = &config.IPIntfNotification{
		IfIndex: testIfIndex,
		IpAddr:  testMyLinkScopeIP,
	}
	testNdpServer.SwitchMac = testSwitchMac
}

func TestNDPStartServer(t *testing.T) {
	InitNDPTestServer()
}

func TestIPv6IntfCreate(t *testing.T) {
	InitNDPTestServer() // event listener channel is already running

	ipv6Obj := &config.IPIntfNotification{
		IfIndex:   testIfIndex,
		IpAddr:    testMyGSIp,
		Operation: config.CONFIG_CREATE,
		IntfRef:   testIntfRef,
	}
	t.Log("Ports Created are:", testNdpServer.PhyPort)
	testNdpServer.HandleIPIntfCreateDelete(ipv6Obj)
	ipv6Obj.IpAddr = testMyLinkScopeIP
	testNdpServer.HandleIPIntfCreateDelete(ipv6Obj)

	t.Log(testNdpServer.L3Port)
	l3Port, exists := testNdpServer.L3Port[testIfIndex]
	if !exists {
		t.Error("failed to init interface")
		return
	}

	if l3Port.IpAddr != testMyGSIp {
		t.Error("failed to set l3 port global scope ip address. wanted:", testMyGSIp, "got:", l3Port.IpAddr)
		return
	}

	if l3Port.globalScope != testMyAbsGSIP {
		t.Error("failed to set l3 port global scope ip address. wanted:", testMyAbsGSIP, "got:", l3Port.globalScope)
		return
	}

	if l3Port.LinkLocalIp != testMyLinkScopeIP {
		t.Error("failed to set l3 port global scope ip address. wanted:", testMyLinkScopeIP, "got:", l3Port.LinkLocalIp)
		return
	}

	if l3Port.linkScope != testMyAbsLinkScopeIP {
		t.Error("failed to set l3 port link scope ip address. wanted:", testMyAbsLinkScopeIP, "got:", l3Port.linkScope)
		return
	}

	if l3Port.PcapBase.PcapUsers != 0 {
		t.Error("pcap users added even when we did not received STATE UP Notification", l3Port.PcapBase.PcapUsers)
		return
	}
}

func TestIPv6IntfDelete(t *testing.T) {
	TestIPv6IntfCreate(t)
	ipv6Obj := &config.IPIntfNotification{
		IfIndex:   testIfIndex,
		IpAddr:    testMyGSIp,
		Operation: config.CONFIG_DELETE,
	}
	testNdpServer.HandleIPIntfCreateDelete(ipv6Obj)

	l3Port, exists := testNdpServer.L3Port[testIfIndex]
	if !exists {
		t.Error("Failed to get L3 Port for ifIndex:", testIfIndex)
		return
	}

	if l3Port.IpAddr != "" {
		t.Error("Failed to delete global scope IP Address:", l3Port.IpAddr)
		return
	}

	if l3Port.globalScope != "" {
		t.Error("Failed to delete global scope IP Address:", l3Port.globalScope)
	}

	ipv6Obj.IpAddr = testMyLinkScopeIP

	testNdpServer.HandleIPIntfCreateDelete(ipv6Obj)

	l3Port, exists = testNdpServer.L3Port[testIfIndex]
	if !exists {
		t.Error("Failed to get L3 Port for ifIndex:", testIfIndex)
		return
	}

	if l3Port.LinkLocalIp != "" {
		t.Error("Failed to delete Link Scope Ip Address:", l3Port.LinkLocalIp)
		return
	}

	if l3Port.linkScope != "" {
		t.Error("Failed to delete link scope iP address:", l3Port.linkScope)
		return
	}
}

func TestIPv6IntfStateUpDown(t *testing.T) {
	TestIPv6IntfCreate(t)
	stateObj := config.IPIntfNotification{
		IfIndex:   testIfIndex,
		Operation: config.STATE_UP,
		IpAddr:    testMyLinkScopeIP,
	}
	t.Log(stateObj)
	testNdpServer.HandleStateNotification(&stateObj)

	l3Port, _ := testNdpServer.L3Port[testIfIndex]
	if l3Port.PcapBase.PcapHandle == nil {
		t.Error("Failed to initialize pcap handler")
		return
	}

	if l3Port.PcapBase.PcapCtrl == nil {
		t.Error("failed to initialize pcap ctrl")
		return
	}

	if l3Port.PcapBase.PcapUsers != 1 {
		t.Error("Failed to add first pcap user")
		return
	}

	stateObj.Operation = config.STATE_UP
	stateObj.IpAddr = testMyGSIp

	t.Log(stateObj)

	testNdpServer.HandleStateNotification(&stateObj)
	l3Port, _ = testNdpServer.L3Port[testIfIndex]
	if l3Port.PcapBase.PcapHandle == nil {
		t.Error("Failed to initialize pcap handler for second time")
		return
	}

	if l3Port.PcapBase.PcapCtrl == nil {
		t.Error("failed to initialize pcap ctrl")
		return
	}

	if l3Port.PcapBase.PcapUsers != 2 {
		t.Error("Failed to add second pcap user")
		return
	}

	stateObj.Operation = config.STATE_DOWN
	stateObj.IpAddr = testMyLinkScopeIP

	testNdpServer.HandleStateNotification(&stateObj)
	l3Port, _ = testNdpServer.L3Port[testIfIndex]
	if l3Port.PcapBase.PcapHandle == nil {
		t.Error("Pcap got deleted even when there was one user")
		return
	}

	if l3Port.PcapBase.PcapCtrl == nil {
		t.Error("Pcap ctrl channel got deleted even when there was one user")
		return
	}

	if l3Port.PcapBase.PcapUsers != 1 {
		t.Error("Failed to delete one pcap user")
		return
	}

	stateObj.Operation = config.STATE_DOWN
	stateObj.IpAddr = testMyGSIp

	testNdpServer.HandleStateNotification(&stateObj)
	l3Port, _ = testNdpServer.L3Port[testIfIndex]
	if l3Port.PcapBase.PcapHandle != nil {
		t.Error("Pcap is not deleted even when there are no users")
		return
	}

	if l3Port.PcapBase.PcapCtrl != nil {
		t.Error("Pcap ctrl channel should be deleted when there are no users")
		return
	}

	if l3Port.PcapBase.PcapUsers != 0 {
		t.Error("Pcap users count should be zero when all ipaddress from interfaces are removed")
		return
	}
}

func TestFindL3Port(t *testing.T) {
	TestIPv6IntfCreate(t)
	if _, exists := testNdpServer.findL3Port(testIfIndex); !exists {
		t.Error("Entry for ifIndex:", testIfIndex, "should exists")
		return
	}
	invalidIfIndex := int32(123)
	if _, exists := testNdpServer.findL3Port(invalidIfIndex); exists {
		t.Error("Entry for ifIndex:", invalidIfIndex, "should not exists")
		return
	}
}

func TestProcessPkt(t *testing.T) {
	TestIPv6IntfCreate(t)

	// NS
	p := gopacket.NewPacket(nsServerBaseTestPkt, layers.LinkTypeEthernet, gopacket.Default)
	if p.ErrorLayer() != nil {
		t.Error("Failed to decode packet:", p.ErrorLayer().Error())
		return
	}
	err := testNdpServer.ProcessRxPkt(testIfIndex, p)
	if err != nil {
		t.Error("Process RX PKT failed:", err)
		return
	}

	//RA
	p = gopacket.NewPacket(raServerBaseTestPkt, layers.LinkTypeEthernet, gopacket.Default)
	if p.ErrorLayer() != nil {
		t.Error("Failed to decode packet:", p.ErrorLayer().Error())
		return
	}
	err = testNdpServer.ProcessRxPkt(testIfIndex, p)
	if err != nil {
		t.Error("Process RX PKT failed:", err)
		return
	}
	//NA
	p = gopacket.NewPacket(naServerBaseTestPkt, layers.LinkTypeEthernet, gopacket.Default)
	if p.ErrorLayer() != nil {
		t.Error("Failed to decode packet:", p.ErrorLayer().Error())
		return
	}
	err = testNdpServer.ProcessRxPkt(testIfIndex, p)
	if err != nil {
		t.Error("Process RX PKT failed:", err)
		return
	}
}

func TestProcessTimerExpiry(t *testing.T) {
	TestIPv6IntfCreate(t)
	pktData := config.PacketData{
		SendPktType: layers.ICMPv6TypeNeighborSolicitation,
		NeighborIp:  testServerNSDstIp,
		NeighborMac: testServerNSDstMac,
		IfIndex:     testIfIndex,
	}

	err := testNdpServer.ProcessTimerExpiry(pktData)
	if err != nil {
		t.Error("Processing Timer expiry failed:", err)
		return
	}
}
