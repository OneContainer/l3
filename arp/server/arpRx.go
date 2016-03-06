package server

import (
        "fmt"
        "net"
        "utils/commonDefs"
        "asicd/asicdConstDefs"
        "github.com/google/gopacket"
        "github.com/google/gopacket/layers"
        "github.com/google/gopacket/pcap"

)

/*
func (server *ARPServer) StartArpRx(port int) {
        portEnt, _ := server.portPropMap[port]
        //var filter string = "not ether proto 0x8809"
        filter := fmt.Sprintln("not ether src", portEnt.MacAddr, "and not ether proto 0x8809")
        server.logger.Info(fmt.Sprintln("Port: ", port, "filter:", filter))
        pcapHdl, err := pcap.OpenLive(portEnt.IfName, server.snapshotLen, server.promiscuous, server.pcapTimeout)
        if pcapHdl == nil {
                server.logger.Info(fmt.Sprintln("Unable to open pcap handler on:", portEnt.IfName, "error:", err))
                return
        } else {
                err := pcapHdl.SetBPFFilter(filter)
                if err != nil {
                        server.logger.Err(fmt.Sprintln("Unable to set filter on port:", port))
                }
        }

        portEnt.PcapHdl = pcapHdl
        server.portPropMap[port] = portEnt
        server.processRxPkts(port)
}
*/

func (server *ARPServer) StartArpRxTx(port int) {
        portEnt, _ := server.portPropMap[port]
        //filter := fmt.Sprintf("not ether src", portEnt.MacAddr, "and not ether proto 0x8809")
        filter := fmt.Sprintf(`not ether src %s`, portEnt.MacAddr)
        filter = filter +  " and not ether proto 0x8809"
        server.logger.Info(fmt.Sprintln("Port: ", port, "Pcap filter:", filter))
        pcapHdl, err := pcap.OpenLive(portEnt.IfName, server.snapshotLen, server.promiscuous, server.pcapTimeout)
        if pcapHdl == nil {
                server.logger.Info(fmt.Sprintln("Unable to open pcap handler on:", portEnt.IfName, "error:", err))
                return
        } else {
                err := pcapHdl.SetBPFFilter(filter)
                if err != nil {
                        server.logger.Err(fmt.Sprintln("Unable to set filter on port:", port))
                }
        }

        portEnt.PcapHdl = pcapHdl
        server.portPropMap[port] = portEnt
        go server.processRxPkts(port)
        server.logger.Info(fmt.Sprintln("Send Arp Probe on port:", port))
        go server.SendArpProbe(port)
}

func (server *ARPServer) processRxPkts(port int) {
        portEnt, _ := server.portPropMap[port]
        src := gopacket.NewPacketSource(portEnt.PcapHdl, layers.LayerTypeEthernet)
        in := src.Packets()
        for {
                select {
                case packet, ok := <-in:
                        if ok {
                                arpLayer := packet.Layer(layers.LayerTypeARP)
                                if arpLayer != nil {
                                        server.processArpPkt(arpLayer, port)
                                } else {
                                        server.processIpPkt(packet, port)
                                }
                        }
                case <-portEnt.CtrlCh:
                        break
                }
        }
        portEnt, _ = server.portPropMap[port]
        portEnt.PcapHdl.Close()
        portEnt.PcapHdl = nil
        server.portPropMap[port] = portEnt
        portEnt.CtrlCh <- true
        return
}

func (server *ARPServer)processArpPkt(arpLayer gopacket.Layer, port int) {
        arp := arpLayer.(*layers.ARP)
        if arp == nil {
                server.logger.Err("Arp layer returns nil")
                return
        }
        portEnt, _ := server.portPropMap[port]
        if portEnt.MacAddr == (net.HardwareAddr(arp.SourceHwAddress)).String() {
                server.logger.Err("Received ARP Packet with our own MAC Address, hence not processing it")
                return
        }

        if arp.Operation == layers.ARPReply {
                server.processArpReply(arp, port)
        } else if arp.Operation == layers.ARPRequest {
                server.processArpRequest(arp, port)
        }
}

func (server *ARPServer)processArpRequest(arp *layers.ARP, port int) {
        srcMac := (net.HardwareAddr(arp.SourceHwAddress)).String()
        srcIp := (net.IP(arp.SourceProtAddress)).String()
        destIp := (net.IP(arp.DstProtAddress)).String()

        /* Check for Local Subnet for SrcIP */
        /* Check for Local Subnet for DestIP */
        if srcIp != "0.0.0.0" {
                portEnt, _ := server.portPropMap[port]
                myIP := net.ParseIP(portEnt.IpAddr)
                mask := portEnt.Netmask
                myNet := myIP.Mask(mask)
                srcIpAddr := net.ParseIP(srcIp)
                srcNet := srcIpAddr.Mask(mask)
                destIpAddr := net.ParseIP(destIp)
                destNet := destIpAddr.Mask(mask)
                if myNet.Equal(srcNet) !=  true ||
                        myNet.Equal(destNet) != true {
                        server.logger.Info(fmt.Sprintln("Received Arp Request but srcIp:", srcIp, " and destIp:", destIp, "are not in same network. Hence, not processing it"))
                        server.logger.Info(fmt.Sprintln("Ip and Netmask on the recvd interface is", myIP, mask))
                        return
                }
        } else {
                portEnt, _ := server.portPropMap[port]
                myIP := net.ParseIP(portEnt.IpAddr)
                mask := portEnt.Netmask
                myNet := myIP.Mask(mask)
                destIpAddr := net.ParseIP(destIp)
                destNet := destIpAddr.Mask(mask)
                if myNet.Equal(destNet) != true {
                        server.logger.Info(fmt.Sprintln("Received Arp Probe but destIp:", destIp, "is not in same network. Hence, not processing it"))
                        server.logger.Info(fmt.Sprintln("Ip and Netmask on the recvd interface is", myIP, mask))
                        return
                }
        }

        //server.logger.Info(fmt.Sprintln("Received Arp Request SrcIP:", srcIp, "SrcMAC: ", srcMac, "DstIP:", destIp))

        srcExist := false
        destExist := false
        portEnt, _ := server.portPropMap[port]
        if portEnt.L3IfIdx != -1 {
                l3Ent, exist := server.l3IntfPropMap[portEnt.L3IfIdx]
                if exist {
                        if srcIp == l3Ent.IpAddr {
                                srcExist = true
                        }
                        if destIp == l3Ent.IpAddr {
                                destExist = true
                        }
                } else {
                        server.logger.Info(fmt.Sprintln("Port:", port, "belong to L3 Interface which doesnot exist"))
                        return
                }
        } else {
                server.logger.Info(fmt.Sprintln("Port:", port, "doesnot belong to L3 Interface"))
                return
        }
        if srcExist == true &&
                destExist == true {
                server.logger.Info(fmt.Sprintln("Received our own gratituous ARP with our own SrcIP:", srcIp, "and destIp:", destIp))
                return
        } else if srcExist != true &&
                destExist != true {
                if srcIp == destIp &&
                        srcIp != "0.0.0.0" {
                        server.logger.Info(fmt.Sprintln("Received Gratuitous Arp with IP:", srcIp))
                        server.logger.Info(fmt.Sprintln("1 Installing Arp entry IP:", srcIp, "MAC:", srcMac))
                        server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                                PortNum: port,
                                IpAddr: srcIp,
                                MacAddr: srcMac,
                        }
                } else {
                        if srcIp == "0.0.0.0" {
                                server.logger.Info(fmt.Sprintln("Received Arp Probe for IP:", destIp))
                                server.logger.Info(fmt.Sprintln("2 Installing Arp entry IP:", destIp, "MAC: incomplete"))
                                server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                                        PortNum: port,
                                        IpAddr: destIp,
                                        MacAddr: "incomplete",
                                }
                        } else {
                                // Arp Request Pkt from neighbor1 for neighbor2 IP
                                server.logger.Info(fmt.Sprintln("Received Arp Request from Neighbor1( IP:", srcIp, "MAC:", srcMac, ") for Neighbor2 (IP:", destIp, "Mac: incomplete)"))

                                server.logger.Info(fmt.Sprintln("3 Installing Arp entry IP:", srcIp, "MAC:", srcMac))
                                server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                                        PortNum: port,
                                        IpAddr: srcIp,
                                        MacAddr: srcMac,
                                }

                                server.logger.Info(fmt.Sprintln("4 Installing Arp entry IP:", destIp, "MAC: incomplete"))
                                server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                                        PortNum: port,
                                        IpAddr: destIp,
                                        MacAddr: "incomplete",
                                }
                        }
                }
        } else if srcExist == true {
                server.logger.Info(fmt.Sprintln("Received our own ARP Request with SrcIP:", srcIp, "DestIP:", destIp))
        } else if destExist == true {
                server.logger.Info(fmt.Sprintln("Received ARP Request for our IP with SrcIP:", srcIp, "DestIP:", destIp, "linux should respond to this request"))
                if srcIp != "0.0.0.0" {
                        server.logger.Info(fmt.Sprintln("5 Installing Arp entry IP:", srcIp, "MAC:", srcMac))
                        server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                                PortNum: port,
                                IpAddr: srcIp,
                                MacAddr: srcMac,
                        }
                } else {
                        server.logger.Info(fmt.Sprintln("Received Arp Probe for IP:", destIp, "linux should respond to this"))
                }
        }
}

func (server *ARPServer) processArpReply(arp *layers.ARP, port int) {
        srcMac := (net.HardwareAddr(arp.SourceHwAddress)).String()
        srcIp := (net.IP(arp.SourceProtAddress)).String()
        //destMac := (net.HardwareAddr(arp.DstHwAddress)).String()
        destIp := (net.IP(arp.DstProtAddress)).String()


        //server.logger.Info(fmt.Sprintln("Received Arp Response SrcIP:", srcIp, "SrcMAC: ", srcMac, "DstIP:", destIp, "DestMac:", destMac))

        if destIp == "0.0.0.0" {
                server.logger.Err(fmt.Sprintln("Recevied Arp reply for ARP Probe and there is a conflicting IP Address:", srcIp))
                return
        }

        /* Check for Local Subnet for SrcIP */
        /* Check for Local Subnet for DestIP */
        portEnt, _ := server.portPropMap[port]
        myIP := net.ParseIP(portEnt.IpAddr)
        mask := portEnt.Netmask
        myNet := myIP.Mask(mask)
        srcIpAddr := net.ParseIP(srcIp)
        srcNet := srcIpAddr.Mask(mask)
        destIpAddr := net.ParseIP(destIp)
        destNet := destIpAddr.Mask(mask)
        if myNet.Equal(srcNet) !=  true ||
                myNet.Equal(destNet) != true {
                server.logger.Info(fmt.Sprintln("Received Arp Reply but srcIp:", srcIp, " and destIp:", destIp, "are not in same network. Hence, not processing it"))
                server.logger.Info(fmt.Sprintln("Netmask on the recvd interface is", mask))
                return
        }
        //server.logger.Info(fmt.Sprintln("6 Installing Arp entry IP:", srcIp, "MAC:", srcMac))
        server.arpEntryUpdateCh <- UpdateArpEntryMsg {
                PortNum: port,
                IpAddr: srcIp,
                MacAddr: srcMac,
        }
}


func (server *ARPServer) processIpPkt(packet gopacket.Packet, port int) {
        if nw := packet.NetworkLayer(); nw != nil {
                sIpAddr, dIpAddr := nw.NetworkFlow().Endpoints()
                dstIp := dIpAddr.String()
                srcIp := sIpAddr.String()

                ethLayer := packet.Layer(layers.LayerTypeEthernet)
                if ethLayer == nil {
                        server.logger.Err("Not an Ethernet frame")
                        return
                }
                eth := ethLayer.(*layers.Ethernet)
                srcMac := (eth.SrcMAC).String()

                l3IntfIdx := server.getL3IntfOnSameSubnet(srcIp)

                if l3IntfIdx != -1 {
                        arpEnt, exist := server.arpCache[srcIp]
                        if exist {
                                vlanEnt, exist := server.vlanPropMap[l3IntfIdx]
                                flag := false
                                if exist {
                                        vlanId := int(asicdConstDefs.GetIntfIdFromIfIndex(int32(l3IntfIdx)))
                                        for p, _ := range vlanEnt.UntagPortMap {
                                                if p == port &&
                                                        arpEnt.VlanId == vlanId {
                                                        flag = true
                                                }
                                        }
                                } else {
                                        flag = false
                                }
                                if !(exist && arpEnt.MacAddr == srcMac &&
                                        port == arpEnt.PortNum && flag == true) {
                                        server.sendArpReqL3Intf(srcIp, l3IntfIdx)
                                }
                        } else {
                                server.sendArpReqL3Intf(srcIp, l3IntfIdx)
                        }
                }

                l3IntfIdx = server.getL3IntfOnSameSubnet(dstIp)
                if l3IntfIdx != -1 {
                        server.sendArpReqL3Intf(dstIp, l3IntfIdx)
                }

        }
}


func (server *ARPServer)sendArpReqL3Intf(ip string, l3IfIdx int) {

        ifType := asicdConstDefs.GetIntfTypeFromIfIndex(int32(l3IfIdx))
        if ifType == commonDefs.L2RefTypeVlan {
                vlanEnt, _ := server.vlanPropMap[l3IfIdx]
                for port, _ := range vlanEnt.UntagPortMap {
                        server.sendArpReq(ip, port)
                }
        } else if ifType == commonDefs.L2RefTypeLag {
                lagEnt, _ := server.lagPropMap[l3IfIdx]
                for port, _ := range lagEnt.PortMap {
                        server.sendArpReq(ip, port)
                }
        } else if ifType == commonDefs.L2RefTypePort {
                server.sendArpReq(ip, l3IfIdx)
        }
}