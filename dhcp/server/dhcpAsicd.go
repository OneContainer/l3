package server

import (
	"asicd/asicdCommonDefs"
	"asicdServices"
	"encoding/json"
	"fmt"
	nanomsg "github.com/op/go-nanomsg"
)

type AsicdClient struct {
	DhcpClientBase
	ClientHdl *asicdServices.ASICDServicesClient
}

func (server *DHCPServer) createASICdSubscriber() {
	for {
		server.logger.Info("Read on ASICd subscriber socket...")
		asicdrxBuf, err := server.asicdSubSocket.Recv(0)
		if err != nil {
			server.logger.Err(fmt.Sprintln("Recv on ASICd subscriber socket failed with error:", err))
			server.asicdSubSocketErrCh <- err
			continue
		}
		server.asicdSubSocketCh <- asicdrxBuf
	}
}

func (server *DHCPServer) listenForASICdUpdates(address string) error {
	var err error
	if server.asicdSubSocket, err = nanomsg.NewSubSocket(); err != nil {
		server.logger.Err(fmt.Sprintln("Failed to create ASICd subscribe socket, error:", err))
		return err
	}

	if err = server.asicdSubSocket.Subscribe(""); err != nil {
		server.logger.Err(fmt.Sprintln("Failed to subscribe to \"\" on ASICd subscribe socket, error:", err))
		return err
	}

	if _, err = server.asicdSubSocket.Connect(address); err != nil {
		server.logger.Err(fmt.Sprintln("Failed to connect to ASICd publisher socket, address:", address, "error:", err))
		return err
	}

	server.logger.Info(fmt.Sprintln("Connected to ASICd publisher at address:", address))
	if err = server.asicdSubSocket.SetRecvBuffer(1024 * 1024); err != nil {
		server.logger.Err(fmt.Sprintln("Failed to set the buffer size for ASICd publisher socket, error:", err))
		return err
	}
	return nil
}

func (server *DHCPServer) processAsicdNotification(asicdrxBuf []byte) {
	var rxMsg asicdCommonDefs.AsicdNotification
	err := json.Unmarshal(asicdrxBuf, &rxMsg)
	if err != nil {
		server.logger.Err(fmt.Sprintln("Unable to unmarshal asicdrxBuf:", asicdrxBuf))
		return
	}
	switch rxMsg.MsgType {
	case asicdCommonDefs.NOTIFY_VLAN_CREATE,
		asicdCommonDefs.NOTIFY_VLAN_UPDATE,
		asicdCommonDefs.NOTIFY_VLAN_DELETE:
		//Vlan Create Msg
		server.logger.Debug("Recvd VLAN notification")
		var vlanMsg asicdCommonDefs.VlanNotifyMsg
		err = json.Unmarshal(rxMsg.Msg, &vlanMsg)
		if err != nil {
			server.logger.Err(fmt.Sprintln("Unable to unmashal vlanNotifyMsg:", rxMsg.Msg))
			return
		}
		server.updateVlanInfra(vlanMsg, rxMsg.MsgType)
	case asicdCommonDefs.NOTIFY_IPV4INTF_CREATE,
		asicdCommonDefs.NOTIFY_IPV4INTF_DELETE:
		server.logger.Debug("Recvd IPV4INTF notification")
		var v4Msg asicdCommonDefs.IPv4IntfNotifyMsg
		err = json.Unmarshal(rxMsg.Msg, &v4Msg)
		if err != nil {
			server.logger.Err(fmt.Sprintln("Unable to unmashal ipv4IntfNotifyMsg:", rxMsg.Msg))
			return
		}
		server.updateIpv4Infra(v4Msg, rxMsg.MsgType)
	case asicdCommonDefs.NOTIFY_L3INTF_STATE_CHANGE:
		//L3_INTF_STATE_CHANGE
		server.logger.Debug("Recvd INTF_STATE_CHANGE notification")
		var l3IntfMsg asicdCommonDefs.L3IntfStateNotifyMsg
		err = json.Unmarshal(rxMsg.Msg, &l3IntfMsg)
		if err != nil {
			server.logger.Err(fmt.Sprintln("Unable to unmashal l3IntfStateNotifyMsg:", rxMsg.Msg))
			return
		}
		//server.processL3StateChange(l3IntfMsg)
	case asicdCommonDefs.NOTIFY_LAG_CREATE,
		asicdCommonDefs.NOTIFY_LAG_UPDATE,
		asicdCommonDefs.NOTIFY_LAG_DELETE:
		server.logger.Debug("Recvd NOTIFY_LAG notification")
		var lagMsg asicdCommonDefs.LagNotifyMsg
		err = json.Unmarshal(rxMsg.Msg, &lagMsg)
		if err != nil {
			server.logger.Err(fmt.Sprintln("Unable to unmashal lagNotifyMsg:", rxMsg.Msg))
			return
		}
		//TODO
		//server.updateLagInfra(lagMsg, rxMsg.MsgType)
	}
}
