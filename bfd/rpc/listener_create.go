package rpc

import (
	"bfdd"
	"errors"
	"fmt"
	"l3/bfd/bfddCommonDefs"
	"l3/bfd/server"
)

func (h *BFDHandler) SendBfdGlobalConfig(bfdGlobalConfig *bfdd.BfdGlobal) bool {
	gConf := server.GlobalConfig{
		Enable: bfdGlobalConfig.Enable,
	}
	h.server.GlobalConfigCh <- gConf
	return true
}

func (h *BFDHandler) SendBfdIntfConfig(bfdIntfConfig *bfdd.BfdInterface) bool {
	ifConf := server.IntfConfig{
		InterfaceId:               bfdIntfConfig.IfIndex,
		LocalMultiplier:           bfdIntfConfig.LocalMultiplier,
		DesiredMinTxInterval:      bfdIntfConfig.DesiredMinTxInterval,
		RequiredMinRxInterval:     bfdIntfConfig.RequiredMinRxInterval,
		RequiredMinEchoRxInterval: bfdIntfConfig.RequiredMinEchoRxInterval,
		DemandEnabled:             bfdIntfConfig.DemandEnabled,
		AuthenticationEnabled:     bfdIntfConfig.AuthenticationEnabled,
		AuthenticationType:        h.server.ConvertBfdAuthTypeStrToVal(bfdIntfConfig.AuthType),
		AuthenticationKeyId:       bfdIntfConfig.AuthKeyId,
		AuthenticationData:        bfdIntfConfig.AuthData,
	}
	h.server.IntfConfigCh <- ifConf
	return true
}

func (h *BFDHandler) SendBfdSessionConfig(bfdSessionConfig *bfdd.BfdSession) bool {
	sessionConf := server.SessionConfig{
		DestIp:    bfdSessionConfig.IpAddr,
		PerLink:   bfdSessionConfig.PerLink,
		Protocol:  bfddCommonDefs.ConvertBfdSessionOwnerStrToVal(bfdSessionConfig.Owner),
		Operation: bfddCommonDefs.CREATE,
	}
	h.server.SessionConfigCh <- sessionConf
	return true
}

func (h *BFDHandler) CreateBfdGlobal(bfdGlobalConf *bfdd.BfdGlobal) (bool, error) {
	if bfdGlobalConf == nil {
		err := errors.New("Invalid Global Configuration")
		return false, err
	}
	h.logger.Info(fmt.Sprintln("Create global config attrs:", bfdGlobalConf))
	return h.SendBfdGlobalConfig(bfdGlobalConf), nil
}

func (h *BFDHandler) CreateBfdInterface(bfdIntfConf *bfdd.BfdInterface) (bool, error) {
	if bfdIntfConf == nil {
		err := errors.New("Invalid Interface Configuration")
		return false, err
	}
	h.logger.Info(fmt.Sprintln("Create interface config attrs:", bfdIntfConf))
	return h.SendBfdIntfConfig(bfdIntfConf), nil
}

func (h *BFDHandler) CreateBfdSession(bfdSessionConf *bfdd.BfdSession) (bool, error) {
	if bfdSessionConf == nil {
		err := errors.New("Invalid Session Configuration")
		return false, err
	}
	h.logger.Info(fmt.Sprintln("Create session config attrs:", bfdSessionConf))
	return h.SendBfdSessionConfig(bfdSessionConf), nil
}
