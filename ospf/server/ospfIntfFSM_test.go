//
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//       Unless required by applicable law or agreed to in writing, software
//       distributed under the License is distributed on an "AS IS" BASIS,
//       WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//       See the License for the specific language governing permissions and
//       limitations under the License.
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
	"l3/ospf/config"
	"testing"
)

func initTestParams() {
	fmt.Println("\n Get Server object")
	ospf = getServerObject()
	initAttr()
	go startDummyChannels(ospf)
}

func TestOspfIntfFSM(t *testing.T) {
	fmt.Println("\n**************** INTF FSM ************\n")
	initTestParams()
	err := intfFSMTestLogic()
	if err != SUCCESS {
		fmt.Println("Failed test case for interface FSM")
	}
}

func intfFSMTestLogic() int {
	ospf.initDefaultIntfConf(key, ipIntfProp, ifType)
	var tNum int

	tNum = 1
	fmt.Println(tNum, ": Running StartOspfIntfFSM")
	ospf.StartOspfIntfFSM(key)

	tNum++
	fmt.Println(tNum, ": Running StartOspfP2PIntfFSM")
	//	ospf.StartOspfP2PIntfFSM(key)

	tNum++
	fmt.Println(tNum, ": Running processNbrDownEvent")
	ospf.processNbrDownEvent(msg, key, false) // broadcast network

	tNum++
	fmt.Println(tNum, ": Running processNbrFullStateMsg")
	ospf.processNbrFullStateMsg(msgNbrFull, key)

	tNum++
	fmt.Println(tNum, ": Running ElectBDR")
	electedBDR, electedRtrId := ospf.ElectBDR(key)
	fmt.Println("Elected BDR ", electedBDR, " electedRtrId ", electedRtrId)

	tNum++
	fmt.Println(tNum, ": Running ElectDR")
	BDR := []byte{10, 1, 1, 2}
	RtrIdBDR := uint32(2)
	dr, drid := ospf.ElectDR(key, BDR, RtrIdBDR)
	fmt.Println("Elected DR ", dr, " Router id ", drid)

	tNum++
	fmt.Println(tNum, ": Running ElectBDRAndDR")
	ospf.IntfConfMap[key] = intf
	ospf.ElectBDRAndDR(key)

	tNum++
	fmt.Println(tNum, ": Running createAndSendEventsIntfFSM")
	oldState := config.Down
	newState := config.DesignatedRouter
	oldRtr := uint32(2)
	oldBdr := uint32(10)
	ospf.createAndSendEventsIntfFSM(key, oldState, newState, oldRtr, oldBdr)

	tNum++
	fmt.Println(tNum, ": Running StopOspfIntfFSM")
	//	ospf.StopOspfIntfFSM(key)
	return SUCCESS
}
