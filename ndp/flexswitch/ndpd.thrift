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
//   This is a auto-generated file, please do not edit!
// _______   __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __  
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  | 
// |  |__   |  |     |  |__   \  V  /     |   (----  \   \/    \/   /  |  |  ---|  |---- |  ,---- |  |__|  | 
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   | 
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  | 
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__| 
//                                                                                                           
		
namespace go ndpd
typedef i32 int
typedef i16 uint16
struct NDPGlobal {
	1 : string Vrf
	2 : string Enable
}
struct NDPEntryState {
	1 : string IpAddr
	2 : string MacAddr
	3 : string Vlan
	4 : string Intf
	5 : string ExpiryTimeLeft
}
struct NDPEntryStateGetInfo {
	1: int StartIdx
	2: int EndIdx
	3: int Count
	4: bool More
	5: list<NDPEntryState> NDPEntryStateList
}

struct PatchOpInfo {
    1 : string Op
    2 : string Path
    3 : string Value
}
			        
service NDPDServices {
	bool CreateNDPGlobal(1: NDPGlobal config);
	bool UpdateNDPGlobal(1: NDPGlobal origconfig, 2: NDPGlobal newconfig, 3: list<bool> attrset, 4: list<PatchOpInfo> op);
	bool DeleteNDPGlobal(1: NDPGlobal config);

	NDPEntryStateGetInfo GetBulkNDPEntryState(1: int fromIndex, 2: int count);
	NDPEntryState GetNDPEntryState(1: string IpAddr);
}