package server

import (
	"asicd/asicdConstDefs"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"l3/bfd/bfddCommonDefs"
	"math/rand"
	"net"
	"strconv"
	"time"
	"utils/commonDefs"
)

func (server *BFDServer) StartSessionHandler() error {
	server.CreateSessionCh = make(chan BfdSessionMgmt)
	server.DeleteSessionCh = make(chan BfdSessionMgmt)
	server.AdminUpSessionCh = make(chan BfdSessionMgmt)
	server.AdminDownSessionCh = make(chan BfdSessionMgmt)
	server.CreatedSessionCh = make(chan int32)
	//go server.StartBfdSessionDiscoverer()
	go server.StartBfdSessionRxTx()
	for {
		select {
		case sessionMgmt := <-server.CreateSessionCh:
			server.CreateBfdSession(sessionMgmt)
		case sessionMgmt := <-server.DeleteSessionCh:
			server.DeleteBfdSession(sessionMgmt)
		case sessionMgmt := <-server.AdminUpSessionCh:
			server.AdminUpBfdSession(sessionMgmt)
		case sessionMgmt := <-server.AdminDownSessionCh:
			server.AdminDownBfdSession(sessionMgmt)

		}
	}
	return nil
}

func (server *BFDServer) StartBfdSessionDiscoverer() error {
	destAddr := ":" + strconv.Itoa(DEST_PORT)
	DefaultListener, err := net.Listen("udp", destAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed Listener creation - ", err))
		return nil
	}
	defer DefaultListener.Close()
	for {
		conn, err := DefaultListener.Accept()
		if err != nil {
			server.logger.Info(fmt.Sprintln("Failed to accept connection - ", err))
		} else {
			server.logger.Info(fmt.Sprintln("Received connection from ", conn.RemoteAddr().String()))
			go server.ReadFromConnection(conn)
		}
	}
	return nil
}

func (server *BFDServer) StartBfdSessionRxTx() error {
	for {
		select {
		case createdSessionId := <-server.CreatedSessionCh:
			session := server.bfdGlobal.Sessions[createdSessionId]
			if session != nil {
				session.TxTimeoutCh = make(chan int32)
				session.SessionTimeoutCh = make(chan int32)
				session.SessionStopClientCh = make(chan bool)
				if session.state.PerLinkSession {
					server.logger.Info(fmt.Sprintln("Starting PerLink server for session ", createdSessionId))
					go session.StartPerLinkSessionServer(server)
					server.logger.Info(fmt.Sprintln("Starting PerLink client for session ", createdSessionId))
					go session.StartPerLinkSessionClient(server)
				} else {
					server.logger.Info(fmt.Sprintln("Starting server for session ", createdSessionId))
					go session.StartSessionServer(server)
					server.logger.Info(fmt.Sprintln("Starting client for session ", createdSessionId))
					go session.StartSessionClient(server)
				}
			} else {
				server.logger.Info(fmt.Sprintf("Bfd session could not be initiated for ", createdSessionId))
			}
		}
	}
	return nil
}

func (server *BFDServer) processSessionConfig(sessionConfig SessionConfig) error {
	sessionMgmt := BfdSessionMgmt{
		DestIp:   sessionConfig.DestIp,
		Protocol: sessionConfig.Protocol,
		PerLink:  sessionConfig.PerLink,
	}
	switch sessionConfig.Operation {
	case bfddCommonDefs.CREATE:
		server.CreateSessionCh <- sessionMgmt
	case bfddCommonDefs.DELETE:
		server.DeleteSessionCh <- sessionMgmt
	case bfddCommonDefs.ADMINUP:
		server.AdminUpSessionCh <- sessionMgmt
	case bfddCommonDefs.ADMINDOWN:
		server.AdminDownSessionCh <- sessionMgmt
	}
	return nil
}

func (server *BFDServer) SendAdminDownToAllNeighbors() error {
	for _, session := range server.bfdGlobal.Sessions {
		session.StopBfdSession()
	}
	return nil
}

func (server *BFDServer) ReadFromConnection(conn net.Conn) error {
	defer conn.Close()
	sessionConf := SessionConfig{
		DestIp:    conn.RemoteAddr().String(),
		PerLink:   false,
		Protocol:  bfddCommonDefs.DISC,
		Operation: bfddCommonDefs.CREATE,
	}
	server.processSessionConfig(sessionConf)
	return nil
}

func (server *BFDServer) GetNewSessionId() int32 {
	var sessionIdUsed bool
	var sessionId int32
	sessionId = 0
	if server.bfdGlobal.NumSessions < MAX_NUM_SESSIONS {
		sessionIdUsed = true //By default assume the sessionId is already used.
		s1 := rand.NewSource(time.Now().UnixNano())
		r1 := rand.New(s1)
		for sessionIdUsed {
			sessionId = r1.Int31n(MAX_NUM_SESSIONS)
			if _, exist := server.bfdGlobal.Sessions[sessionId]; exist {
				server.logger.Info(fmt.Sprintln("GetNewSessionId: sessionId ", sessionId, " is in use, Generating a new one"))
			} else {
				sessionIdUsed = false
			}
		}
	}
	return sessionId
}

func (server *BFDServer) GetIfIndexAndLocalIpFromDestIp(DestIp string) (int32, string) {
	reachabilityInfo, err := server.ribdClient.ClientHdl.GetRouteReachabilityInfo(DestIp)
	if err != nil {
		server.logger.Info(fmt.Sprintf("%s is not reachable", DestIp))
		return int32(0), ""
	}
	ifIndex := asicdConstDefs.GetIfIndexFromIntfIdAndIntfType(int(reachabilityInfo.NextHopIfIndex), int(reachabilityInfo.NextHopIfType))
	server.logger.Info(fmt.Sprintln("GetIfIndexAndLocalIpFromDestIp: DestIp: ", DestIp, "IfIndex: ", ifIndex))
	return ifIndex, reachabilityInfo.NextHopIp
}

func (server *BFDServer) GetTxJitter() int32 {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	jitter := r1.Int31n(TX_JITTER)
	return jitter
}

func (server *BFDServer) NewNormalBfdSession(IfIndex int32, DestIp string, PerLink bool, Protocol bfddCommonDefs.BfdSessionOwner) *BfdSession {
	bfdSession := &BfdSession{}
	sessionId := server.GetNewSessionId()
	if sessionId == 0 {
		server.logger.Info("Failed to get sessionId")
		return nil
	}
	bfdSession.state.SessionId = sessionId
	bfdSession.state.RemoteIpAddr = DestIp
	bfdSession.state.InterfaceId = IfIndex
	bfdSession.state.PerLinkSession = PerLink
	if PerLink {
		IfName, _ := server.getLinuxIntfName(IfIndex)
		bfdSession.state.LocalMacAddr, _ = server.getMacAddrFromIntfName(IfName)
		bfdSession.state.RemoteMacAddr, _ = net.ParseMAC(bfdDedicatedMac)
		bfdSession.useDedicatedMac = true
	}
	bfdSession.state.RegisteredProtocols = make([]bool, bfddCommonDefs.MAX_NUM_PROTOCOLS)
	bfdSession.state.RegisteredProtocols[Protocol] = true
	bfdSession.state.SessionState = STATE_DOWN
	bfdSession.state.RemoteSessionState = STATE_DOWN
	bfdSession.state.LocalDiscriminator = uint32(bfdSession.state.SessionId)
	bfdSession.state.LocalDiagType = DIAG_NONE
	intf, exist := server.bfdGlobal.Interfaces[bfdSession.state.InterfaceId]
	if exist {
		bfdSession.txInterval = STARTUP_TX_INTERVAL / 1000
		bfdSession.txJitter = server.GetTxJitter()
		bfdSession.rxInterval = (STARTUP_RX_INTERVAL * intf.conf.LocalMultiplier) / 1000
		bfdSession.state.LocalIpAddr = intf.property.IpAddr.String()
		bfdSession.state.DesiredMinTxInterval = intf.conf.DesiredMinTxInterval
		bfdSession.state.RequiredMinRxInterval = intf.conf.RequiredMinRxInterval
		bfdSession.state.DetectionMultiplier = intf.conf.LocalMultiplier
		bfdSession.state.DemandMode = intf.conf.DemandEnabled
		bfdSession.authEnabled = intf.conf.AuthenticationEnabled
		bfdSession.authType = AuthenticationType(intf.conf.AuthenticationType)
		bfdSession.authSeqNum = 1
		bfdSession.authKeyId = uint32(intf.conf.AuthenticationKeyId)
		bfdSession.authData = intf.conf.AuthenticationData
	}
	bfdSession.server = server
	bfdSession.bfdPacket = NewBfdControlPacketDefault()
	server.bfdGlobal.Sessions[sessionId] = bfdSession
	server.bfdGlobal.Interfaces[IfIndex].NumSessions++
	server.bfdGlobal.NumSessions++
	server.bfdGlobal.SessionsIdSlice = append(server.bfdGlobal.SessionsIdSlice, sessionId)
	server.logger.Info(fmt.Sprintln("New session : ", sessionId, " created on : ", IfIndex))
	server.CreatedSessionCh <- sessionId
	return bfdSession
}

func (server *BFDServer) NewPerLinkBfdSessions(IfIndex int32, DestIp string, Protocol bfddCommonDefs.BfdSessionOwner) error {
	lag, exist := server.lagPropertyMap[IfIndex]
	if exist {
		for _, link := range lag.Links {
			bfdSession := server.NewNormalBfdSession(link, DestIp, true, Protocol)
			if bfdSession == nil {
				server.logger.Info(fmt.Sprintln("Failed to create perlink session on ", link))
			}
		}
	} else {
		server.logger.Info(fmt.Sprintln("Unknown lag ", IfIndex, " can not create perlink sessions"))
	}
	return nil
}

func (server *BFDServer) NewBfdSession(DestIp string, Protocol bfddCommonDefs.BfdSessionOwner, PerLink bool) *BfdSession {
	IfIndex, _ := server.GetIfIndexAndLocalIpFromDestIp(DestIp)
	if IfIndex == 0 {
		server.logger.Info(fmt.Sprintln("RemoteIP ", DestIp, " is not reachable"))
		return nil
	}
	IfType := asicdConstDefs.GetIntfTypeFromIfIndex(IfIndex)
	intf, exist := server.bfdGlobal.Interfaces[IfIndex]
	if exist {
		if intf.Enabled {
			if IfType == commonDefs.L2RefTypeLag && PerLink {
				server.NewPerLinkBfdSessions(IfIndex, DestIp, Protocol)
			} else {
				bfdSession := server.NewNormalBfdSession(IfIndex, DestIp, false, Protocol)
				return bfdSession
			}
		} else {
			server.logger.Info(fmt.Sprintln("Bfd not enabled on interface ", IfIndex))
		}
	} else {
		server.logger.Info(fmt.Sprintln("Unknown interface ", IfIndex))
	}
	return nil
}

func (server *BFDServer) UpdateBfdSessionsOnInterface(ifIndex int32) error {
	intf, exist := server.bfdGlobal.Interfaces[ifIndex]
	if exist {
		intfEnabled := intf.Enabled
		for _, session := range server.bfdGlobal.Sessions {
			if session.state.InterfaceId == ifIndex {
				session.state.LocalIpAddr = intf.property.IpAddr.String()
				session.state.DesiredMinTxInterval = intf.conf.DesiredMinTxInterval
				session.state.RequiredMinRxInterval = intf.conf.RequiredMinRxInterval
				session.state.DetectionMultiplier = intf.conf.LocalMultiplier
				session.state.DemandMode = intf.conf.DemandEnabled
				session.authEnabled = intf.conf.AuthenticationEnabled
				session.authType = AuthenticationType(intf.conf.AuthenticationType)
				session.authKeyId = uint32(intf.conf.AuthenticationKeyId)
				session.authData = intf.conf.AuthenticationData
				if intfEnabled {
					session.InitiatePollSequence()
				} else {
					session.StopBfdSession()
				}
			}
		}
	}
	return nil
}

func (server *BFDServer) FindBfdSession(DestIp string) (sessionId int32, found bool) {
	found = false
	for sessionId, session := range server.bfdGlobal.Sessions {
		if session.state.RemoteIpAddr == DestIp {
			return sessionId, true
		}
	}
	return sessionId, found
}

func NewBfdControlPacketDefault() *BfdControlPacket {
	bfdControlPacket := &BfdControlPacket{
		Version:    DEFAULT_BFD_VERSION,
		Diagnostic: DIAG_NONE,
		State:      STATE_DOWN,
		Poll:       false,
		Final:      false,
		ControlPlaneIndependent:   false,
		AuthPresent:               false,
		Demand:                    false,
		Multipoint:                false,
		DetectMult:                DEFAULT_DETECT_MULTI,
		MyDiscriminator:           0,
		YourDiscriminator:         0,
		DesiredMinTxInterval:      DEFAULT_DESIRED_MIN_TX_INTERVAL,
		RequiredMinRxInterval:     DEFAULT_REQUIRED_MIN_RX_INTERVAL,
		RequiredMinEchoRxInterval: DEFAULT_REQUIRED_MIN_ECHO_RX_INTERVAL,
		AuthHeader:                nil,
	}
	return bfdControlPacket
}

// CreateBfdSession initializes a session and starts cpntrol packets exchange.
// This function is called when a protocol registers with BFD to monitor a destination IP.
func (server *BFDServer) CreateBfdSession(sessionMgmt BfdSessionMgmt) (*BfdSession, error) {
	var bfdSession *BfdSession
	DestIp := sessionMgmt.DestIp
	Protocol := sessionMgmt.Protocol
	PerLink := sessionMgmt.PerLink
	sessionId, found := server.FindBfdSession(DestIp)
	if !found {
		server.logger.Info(fmt.Sprintln("CreateSession ", DestIp, Protocol, PerLink))
		bfdSession = server.NewBfdSession(DestIp, Protocol, PerLink)
		if bfdSession != nil {
			//server.bfdGlobal.Sessions[bfdSession.state.SessionId] = bfdSession
			server.logger.Info(fmt.Sprintln("Bfd session created ", bfdSession.state.SessionId, bfdSession.state.RemoteIpAddr))
		} else {
			server.logger.Info(fmt.Sprintln("CreateSession failed for ", DestIp, Protocol))
		}
	} else {
		server.logger.Info(fmt.Sprintln("Bfd session already exists ", DestIp, Protocol, sessionId))
		bfdSession = server.bfdGlobal.Sessions[sessionId]
		if !bfdSession.state.RegisteredProtocols[Protocol] {
			bfdSession.state.RegisteredProtocols[Protocol] = true
		}
	}
	return bfdSession, nil
}

func (server *BFDServer) SessionDeleteHandler(session *BfdSession, Protocol bfddCommonDefs.BfdSessionOwner) error {
	var i int
	sessionId := session.state.SessionId
	session.state.RegisteredProtocols[Protocol] = false
	if session.CheckIfAnyProtocolRegistered() == false {
		session.txTimer.Stop()
		session.sessionTimer.Stop()
		session.SessionStopClientCh <- true
		server.bfdGlobal.Interfaces[session.state.InterfaceId].NumSessions--
		server.bfdGlobal.NumSessions--
		delete(server.bfdGlobal.Sessions, sessionId)
		for i = 0; i < len(server.bfdGlobal.SessionsIdSlice); i++ {
			if server.bfdGlobal.SessionsIdSlice[i] == sessionId {
				break
			}
		}
		server.bfdGlobal.SessionsIdSlice = append(server.bfdGlobal.SessionsIdSlice[:i], server.bfdGlobal.SessionsIdSlice[i+1:]...)
	}
	return nil
}

func (server *BFDServer) DeletePerLinkSessions(DestIp string, Protocol bfddCommonDefs.BfdSessionOwner) error {
	for _, session := range server.bfdGlobal.Sessions {
		if session.state.RemoteIpAddr == DestIp {
			server.SessionDeleteHandler(session, Protocol)
		}
	}
	return nil
}

// DeleteBfdSession ceases the session.
// A session down control packet is sent to BFD neighbor before deleting the session.
// This function is called when a protocol decides to stop monitoring the destination IP.
func (server *BFDServer) DeleteBfdSession(sessionMgmt BfdSessionMgmt) error {
	DestIp := sessionMgmt.DestIp
	Protocol := sessionMgmt.Protocol
	server.logger.Info(fmt.Sprintln("DeleteSession ", DestIp, Protocol))
	sessionId, found := server.FindBfdSession(DestIp)
	if found {
		session := server.bfdGlobal.Sessions[sessionId]
		if session.state.PerLinkSession {
			server.DeletePerLinkSessions(DestIp, Protocol)
		} else {
			server.SessionDeleteHandler(session, Protocol)
		}
	} else {
		server.logger.Info(fmt.Sprintln("Bfd session not found ", sessionId))
	}
	return nil
}

func (server *BFDServer) AdminUpPerLinkBfdSessions(DestIp string) error {
	for _, session := range server.bfdGlobal.Sessions {
		if session.state.RemoteIpAddr == DestIp {
			session.StartBfdSession()
		}
	}
	return nil
}

// AdminUpBfdSession ceases the session.
func (server *BFDServer) AdminUpBfdSession(sessionMgmt BfdSessionMgmt) error {
	DestIp := sessionMgmt.DestIp
	Protocol := sessionMgmt.Protocol
	server.logger.Info(fmt.Sprintln("AdminDownSession ", DestIp, Protocol))
	sessionId, found := server.FindBfdSession(DestIp)
	if found {
		session := server.bfdGlobal.Sessions[sessionId]
		if session.state.PerLinkSession {
			server.AdminUpPerLinkBfdSessions(DestIp)
		} else {
			server.bfdGlobal.Sessions[sessionId].StartBfdSession()
		}
	} else {
		server.logger.Info(fmt.Sprintln("Bfd session not found ", sessionId))
	}
	return nil
}

func (server *BFDServer) AdminDownPerLinkBfdSessions(DestIp string) error {
	for _, session := range server.bfdGlobal.Sessions {
		if session.state.RemoteIpAddr == DestIp {
			session.StopBfdSession()
		}
	}
	return nil
}

// AdminDownBfdSession ceases the session.
func (server *BFDServer) AdminDownBfdSession(sessionMgmt BfdSessionMgmt) error {
	DestIp := sessionMgmt.DestIp
	Protocol := sessionMgmt.Protocol
	server.logger.Info(fmt.Sprintln("AdminDownSession ", DestIp, Protocol))
	sessionId, found := server.FindBfdSession(DestIp)
	if found {
		session := server.bfdGlobal.Sessions[sessionId]
		if session.state.PerLinkSession {
			server.AdminDownPerLinkBfdSessions(DestIp)
		} else {
			server.bfdGlobal.Sessions[sessionId].StopBfdSession()
		}
	} else {
		server.logger.Info(fmt.Sprintln("Bfd session not found ", sessionId))
	}
	return nil
}

// This function handles NextHop change from RIB.
// Subsequent control packets will be sent using the BFD attributes configuration on the new IfIndex.
// A Poll control packet will be sent to BFD neighbor and expact a Final control packet.
func (server *BFDServer) HandleNextHopChange(DestIp string) error {
	return nil
}

func (session *BfdSession) StartSessionServer(server *BFDServer) error {
	destAddr := session.state.LocalIpAddr + ":" + strconv.Itoa(DEST_PORT)
	ServerAddr, err := net.ResolveUDPAddr("udp", destAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed ResolveUDPAddr ", destAddr, err))
		return nil
	}
	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed ListenUDP ", err))
		return nil
	}
	sessionId := session.state.SessionId
	defer ServerConn.Close()
	buf := make([]byte, 1024)
	for {
		if server.bfdGlobal.Sessions[sessionId] == nil {
			return nil
		}
		len, _, err := ServerConn.ReadFromUDP(buf)
		if err != nil {
			server.logger.Info(fmt.Sprintln("Failed to read from ", ServerAddr))
		} else {
			if len >= DEFAULT_CONTROL_PACKET_LEN {
				bfdPacket, err := DecodeBfdControlPacket(buf[0:len])
				if err == nil {
					session.state.NumRxPackets++
					session.ProcessBfdPacket(bfdPacket)
				} else {
					server.logger.Info(fmt.Sprintln("Failed to decode packet - ", err))
				}
			}
		}
	}
	return nil
}

func (session *BfdSession) CanProcessBfdControlPacket(bfdPacket *BfdControlPacket) bool {
	var canProcess bool
	canProcess = true
	if bfdPacket.Version != DEFAULT_BFD_VERSION {
		canProcess = false
		fmt.Sprintln("Can't process version mismatch ", bfdPacket.Version, DEFAULT_BFD_VERSION)
	}
	if bfdPacket.DetectMult == 0 {
		canProcess = false
		fmt.Sprintln("Can't process detect multi ", bfdPacket.DetectMult)
	}
	if bfdPacket.Multipoint {
		canProcess = false
		fmt.Sprintln("Can't process Multipoint ", bfdPacket.Multipoint)
	}
	if bfdPacket.MyDiscriminator == 0 {
		canProcess = false
		fmt.Sprintln("Can't process remote discriminator ", bfdPacket.MyDiscriminator)
	}
	/*
		if bfdPacket.YourDiscriminator == 0 {
			canProcess = false
			fmt.Sprintln("Can't process local discriminator ", bfdPacket.YourDiscriminator)
		} else {
			sessionId := bfdPacket.YourDiscriminator
			session := server.bfdGlobal.Sessions[int32(sessionId)]
			if session != nil {
				if session.state.SessionState == STATE_ADMIN_DOWN {
					canProcess = false
				}
			}
		}
	*/
	return canProcess
}

func (session *BfdSession) AuthenticateReceivedControlPacket(bfdPacket *BfdControlPacket) bool {
	var authenticated bool
	if !bfdPacket.AuthPresent {
		authenticated = true
	} else {
		copiedPacket := &BfdControlPacket{}
		*copiedPacket = *bfdPacket
		authType := bfdPacket.AuthHeader.Type
		keyId := uint32(bfdPacket.AuthHeader.AuthKeyID)
		authData := bfdPacket.AuthHeader.AuthData
		seqNum := bfdPacket.AuthHeader.SequenceNumber
		if authType == session.authType {
			if authType == BFD_AUTH_TYPE_SIMPLE {
				fmt.Sprintln("Authentication type simple: keyId, authData ", keyId, string(authData))
				if keyId == session.authKeyId && string(authData) == session.authData {
					authenticated = true
				}
			} else {
				if seqNum >= session.state.ReceivedAuthSeq && keyId == session.authKeyId {
					var binBuf bytes.Buffer
					copiedPacket.AuthHeader.AuthData = []byte(session.authData)
					binary.Write(&binBuf, binary.BigEndian, copiedPacket)
					switch authType {
					case BFD_AUTH_TYPE_KEYED_MD5, BFD_AUTH_TYPE_METICULOUS_MD5:
						var authDataSum [16]byte
						authDataSum = md5.Sum(binBuf.Bytes())
						if bytes.Equal(authData[:], authDataSum[:]) {
							authenticated = true
						} else {
							fmt.Sprintln("Authentication data did't match for type: ", authType)
						}
					case BFD_AUTH_TYPE_KEYED_SHA1, BFD_AUTH_TYPE_METICULOUS_SHA1:
						var authDataSum [20]byte
						authDataSum = sha1.Sum(binBuf.Bytes())
						if bytes.Equal(authData[:], authDataSum[:]) {
							authenticated = true
						} else {
							fmt.Sprintln("Authentication data did't match for type: ", authType)
						}
					}
				} else {
					fmt.Sprintln("Sequence number and key id check failed: ", seqNum, session.state.ReceivedAuthSeq, keyId, session.authKeyId)
				}
			}
		} else {
			fmt.Sprintln("Authentication type did't match: ", authType, session.authType)
		}
	}
	return authenticated
}

func (session *BfdSession) ProcessBfdPacket(bfdPacket *BfdControlPacket) error {
	var event BfdSessionEvent
	authenticated := session.AuthenticateReceivedControlPacket(bfdPacket)
	if authenticated == false {
		session.server.logger.Info(fmt.Sprintln("Can't authenticatereceived bfd packet for session ", session.state.SessionId))
		return nil
	}
	canProcess := session.CanProcessBfdControlPacket(bfdPacket)
	if canProcess == false {
		session.server.logger.Info(fmt.Sprintln("Can't process received bfd packet for session ", session.state.SessionId))
		return nil
	}
	session.state.RemoteSessionState = bfdPacket.State
	session.state.RemoteDiscriminator = bfdPacket.MyDiscriminator
	session.state.RemoteMinRxInterval = int32(bfdPacket.RequiredMinRxInterval)
	session.rxInterval = (int32(bfdPacket.DesiredMinTxInterval) * int32(bfdPacket.DetectMult)) / 1000
	switch session.state.RemoteSessionState {
	case STATE_DOWN:
		event = REMOTE_DOWN
		session.state.LocalDiagType = DIAG_NEIGHBOR_SIGNAL_DOWN
	case STATE_INIT:
		event = REMOTE_INIT
	case STATE_UP:
		event = REMOTE_UP
		if session.state.SessionState == STATE_UP {
			session.txInterval = session.state.DesiredMinTxInterval / 1000
		}
	case STATE_ADMIN_DOWN:
		event = REMOTE_ADMIN_DOWN
	}
	session.EventHandler(event)
	session.RemoteChangedDemandMode(bfdPacket)
	session.ProcessPollSequence(bfdPacket)
	session.sessionTimer.Stop()
	if session.state.SessionState != STATE_ADMIN_DOWN &&
		session.state.RemoteSessionState != STATE_ADMIN_DOWN {
		sessionTimeoutMS := time.Duration(session.rxInterval)
		session.sessionTimer = time.AfterFunc(time.Millisecond*sessionTimeoutMS, func() { session.SessionTimeoutCh <- session.state.SessionId })
	}
	return nil
}

func (session *BfdSession) UpdateBfdSessionControlPacket() error {
	session.bfdPacket.Diagnostic = session.state.LocalDiagType
	session.bfdPacket.State = session.state.SessionState
	session.bfdPacket.DetectMult = uint8(session.state.DetectionMultiplier)
	session.bfdPacket.MyDiscriminator = session.state.LocalDiscriminator
	session.bfdPacket.YourDiscriminator = session.state.RemoteDiscriminator
	session.bfdPacket.RequiredMinRxInterval = time.Duration(session.state.RequiredMinRxInterval)
	if session.state.SessionState == STATE_UP && session.state.RemoteSessionState == STATE_UP {
		session.bfdPacket.DesiredMinTxInterval = time.Duration(session.state.DesiredMinTxInterval)
		wasDemand := session.bfdPacket.Demand
		session.bfdPacket.Demand = session.state.DemandMode
		isDemand := session.bfdPacket.Demand
		if !wasDemand && isDemand {
			fmt.Sprintln("Enabled demand for session ", session.state.SessionId)
			session.sessionTimer.Stop()
		}
		if wasDemand && !isDemand {
			fmt.Sprintln("Disabled demand for session ", session.state.SessionId)
			sessionTimeoutMS := time.Duration(session.rxInterval)
			session.sessionTimer = time.AfterFunc(time.Millisecond*sessionTimeoutMS, func() { session.SessionTimeoutCh <- session.state.SessionId })
		}
	} else {
		session.bfdPacket.DesiredMinTxInterval = time.Duration(STARTUP_TX_INTERVAL)
	}
	session.bfdPacket.Poll = session.pollSequence
	session.bfdPacket.Final = session.pollSequenceFinal
	session.pollSequenceFinal = false
	if session.authEnabled {
		session.bfdPacket.AuthPresent = true
		session.bfdPacket.AuthHeader.Type = session.authType
		if session.authType != BFD_AUTH_TYPE_SIMPLE {
			session.bfdPacket.AuthHeader.SequenceNumber = session.authSeqNum
		}
		if session.authType == BFD_AUTH_TYPE_METICULOUS_MD5 || session.authType == BFD_AUTH_TYPE_METICULOUS_SHA1 {
			session.authSeqNum++
		}
		session.bfdPacket.AuthHeader.AuthKeyID = uint8(session.authKeyId)
		session.bfdPacket.AuthHeader.AuthData = []byte(session.authData)
	} else {
		session.bfdPacket.AuthPresent = false
	}
	return nil
}

func (session *BfdSession) CheckIfAnyProtocolRegistered() bool {
	for i := bfddCommonDefs.BfdSessionOwner(1); i < bfddCommonDefs.MAX_NUM_PROTOCOLS; i++ {
		if session.state.RegisteredProtocols[i] == true {
			return true
		}
	}
	return false
}

// Stop session as Bfd is disabled globally. Do not delete
func (session *BfdSession) StopBfdSession() error {
	session.EventHandler(ADMIN_DOWN)
	session.state.LocalDiagType = DIAG_ADMIN_DOWN
	session.txTimer.Stop()
	session.sessionTimer.Stop()
	return nil
}

func (session *BfdSession) GetBfdSessionNotification() bool {
	var bfdState bool
	bfdState = false
	if session.state.SessionState == STATE_UP ||
		session.state.SessionState == STATE_ADMIN_DOWN ||
		session.state.RemoteSessionState == STATE_ADMIN_DOWN {
		bfdState = true
	}
	return bfdState
}

func (session *BfdSession) SendBfdNotification() error {
	bfdState := session.GetBfdSessionNotification()
	bfdNotification := bfddCommonDefs.BfddNotifyMsg{
		DestIp: session.state.RemoteIpAddr,
		State:  bfdState,
	}
	bfdNotificationBuf, err := json.Marshal(bfdNotification)
	if err != nil {
		session.server.logger.Err(fmt.Sprintln("Failed to marshal BfdSessionNotification message for session ", session.state.SessionId))
	}
	session.server.notificationCh <- bfdNotificationBuf
	return nil
}

// Restart session that was stopped earlier due to global Bfd disable.
func (session *BfdSession) StartBfdSession() error {
	sessionTimeoutMS := time.Duration(session.rxInterval)
	txTimerMS := time.Duration(session.txInterval)
	session.sessionTimer = time.AfterFunc(time.Millisecond*sessionTimeoutMS, func() { session.SessionTimeoutCh <- session.state.SessionId })
	session.txTimer = time.AfterFunc(time.Millisecond*txTimerMS, func() { session.TxTimeoutCh <- session.state.SessionId })
	session.state.SessionState = STATE_DOWN
	session.EventHandler(ADMIN_UP)
	return nil
}

/* State Machine
                             +--+
                             |  | UP, TIMER
                             |  V
                     DOWN  +------+  INIT
              +------------|      |------------+
              |            | DOWN |            |
              |  +-------->|      |<--------+  |
              |  |         +------+         |  |
              |  |                          |  |
              |  |                          |  |
              |  |                     DOWN,|  |
              |  |TIMER                TIMER|  |
              V  |                          |  V
            +------+                      +------+
       +----|      |                      |      |----+
   DOWN|    | INIT |--------------------->|  UP  |    |INIT, UP
       +--->|      | INIT, UP             |      |<---+
            +------+                      +------+
*/
// EventHandler is called after receiving a BFD packet from remote.
func (session *BfdSession) EventHandler(event BfdSessionEvent) error {
	switch session.state.SessionState {
	case STATE_ADMIN_DOWN:
		fmt.Printf("Received %d event in ADMINDOWN state\n", event)
	case STATE_DOWN:
		switch event {
		case REMOTE_DOWN:
			session.MoveToInitState()
		case REMOTE_INIT:
			session.MoveToUpState()
		case ADMIN_UP:
			session.MoveToDownState()
		case ADMIN_DOWN:
			session.LocalAdminDown()
		case REMOTE_ADMIN_DOWN:
			session.RemoteAdminDown()
		case TIMEOUT, REMOTE_UP:
		}
	case STATE_INIT:
		switch event {
		case REMOTE_INIT, REMOTE_UP:
			session.MoveToUpState()
		case TIMEOUT:
			session.MoveToDownState()
		case ADMIN_DOWN:
			session.LocalAdminDown()
		case REMOTE_ADMIN_DOWN:
			session.RemoteAdminDown()
		case REMOTE_DOWN, ADMIN_UP:
		}
	case STATE_UP:
		switch event {
		case REMOTE_DOWN, TIMEOUT:
			session.MoveToDownState()
		case ADMIN_DOWN:
			session.LocalAdminDown()
		case REMOTE_ADMIN_DOWN:
			session.RemoteAdminDown()
		case REMOTE_INIT, REMOTE_UP, ADMIN_UP:
		}
	}
	return nil
}

func (session *BfdSession) LocalAdminDown() error {
	session.state.SessionState = STATE_ADMIN_DOWN
	session.SendBfdNotification()
	session.txInterval = STARTUP_TX_INTERVAL / 1000
	session.rxInterval = (STARTUP_RX_INTERVAL * session.state.DetectionMultiplier) / 1000
	session.sessionTimer.Stop()
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) RemoteAdminDown() error {
	session.state.RemoteSessionState = STATE_ADMIN_DOWN
	session.state.LocalDiagType = DIAG_NEIGHBOR_SIGNAL_DOWN
	session.SendBfdNotification()
	session.txInterval = STARTUP_TX_INTERVAL / 1000
	session.rxInterval = (STARTUP_RX_INTERVAL * session.state.DetectionMultiplier) / 1000
	session.sessionTimer.Stop()
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) MoveToDownState() error {
	session.state.SessionState = STATE_DOWN
	if session.authType == BFD_AUTH_TYPE_KEYED_MD5 || session.authType == BFD_AUTH_TYPE_KEYED_SHA1 {
		session.authSeqNum++
	}
	session.useDedicatedMac = true
	session.SendBfdNotification()
	session.txInterval = STARTUP_TX_INTERVAL / 1000
	session.rxInterval = (STARTUP_RX_INTERVAL * session.state.DetectionMultiplier) / 1000
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) MoveToInitState() error {
	session.state.SessionState = STATE_INIT
	session.useDedicatedMac = true
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) MoveToUpState() error {
	session.state.SessionState = STATE_UP
	session.state.LocalDiagType = DIAG_NONE
	session.SendBfdNotification()
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) ApplyTxJitter() time.Duration {
	txInterval := session.txInterval * (1 - session.txJitter/100)
	return time.Duration(txInterval)
}

func (session *BfdSession) StartSessionClient(server *BFDServer) error {
	destAddr := session.state.RemoteIpAddr + ":" + strconv.Itoa(DEST_PORT)
	ServerAddr, err := net.ResolveUDPAddr("udp", destAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed ResolveUDPAddr ", destAddr, err))
	}
	localAddr := session.state.LocalIpAddr + ":" + strconv.Itoa(SRC_PORT)
	ClientAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed ResolveUDPAddr ", localAddr, err))
	}
	Conn, err := net.DialUDP("udp", ClientAddr, ServerAddr)
	if err != nil {
		server.logger.Info(fmt.Sprintln("Failed DialUDP ", ClientAddr, ServerAddr, err))
	}
	sessionTimeoutMS := time.Duration(session.rxInterval)
	txTimerMS := time.Duration(session.txInterval)
	session.sessionTimer = time.AfterFunc(time.Millisecond*sessionTimeoutMS, func() { session.SessionTimeoutCh <- session.state.SessionId })
	session.txTimer = time.AfterFunc(time.Millisecond*txTimerMS, func() { session.TxTimeoutCh <- session.state.SessionId })
	defer Conn.Close()
	for {
		select {
		case sessionId := <-session.TxTimeoutCh:
			bfdSession := server.bfdGlobal.Sessions[sessionId]
			bfdSession.UpdateBfdSessionControlPacket()
			buf, err := bfdSession.bfdPacket.CreateBfdControlPacket()
			if err != nil {
				server.logger.Info(fmt.Sprintln("Failed to create control packet for session ", bfdSession.state.SessionId))
			} else {
				_, err = Conn.Write(buf)
				if err != nil {
					server.logger.Info(fmt.Sprintln("failed to send control packet for session ", bfdSession.state.SessionId))
				} else {
					bfdSession.state.NumTxPackets++
				}
				bfdSession.txTimer.Stop()
				if session.state.SessionState != STATE_ADMIN_DOWN &&
					session.state.RemoteSessionState != STATE_ADMIN_DOWN {
					txTimerMS = bfdSession.ApplyTxJitter()
					bfdSession.txTimer = time.AfterFunc(time.Millisecond*txTimerMS, func() { bfdSession.TxTimeoutCh <- bfdSession.state.SessionId })
				}
			}
		case sessionId := <-session.SessionTimeoutCh:
			bfdSession := server.bfdGlobal.Sessions[sessionId]
			server.logger.Info(fmt.Sprintln("Timer expired for : ", sessionId, bfdSession.state.SessionState, bfdSession.rxInterval))
			bfdSession.state.LocalDiagType = DIAG_TIME_EXPIRED
			bfdSession.EventHandler(TIMEOUT)
			bfdSession.sessionTimer.Stop()
			sessionTimeoutMS = time.Duration(bfdSession.rxInterval)
			bfdSession.sessionTimer = time.AfterFunc(time.Millisecond*sessionTimeoutMS, func() { bfdSession.SessionTimeoutCh <- bfdSession.state.SessionId })
		case <-session.SessionStopClientCh:
			return nil
		}
	}
}

func (session *BfdSession) RemoteChangedDemandMode(bfdPacket *BfdControlPacket) error {
	var wasDemandMode, isDemandMode bool
	wasDemandMode = session.state.RemoteDemandMode
	session.state.RemoteDemandMode = bfdPacket.Demand
	if session.state.RemoteDemandMode {
		isDemandMode = true
		session.txTimer.Stop()
	}
	if wasDemandMode && !isDemandMode {
		txTimerMS := time.Duration(session.txInterval)
		session.txTimer = time.AfterFunc(time.Millisecond*txTimerMS, func() { session.TxTimeoutCh <- session.state.SessionId })
	}
	return nil
}

func (session *BfdSession) InitiatePollSequence() error {
	session.server.logger.Info(fmt.Sprintln("Starting poll sequence for session ", session.state.SessionId))
	session.pollSequence = true
	session.txTimer.Reset(0)
	return nil
}

func (session *BfdSession) ProcessPollSequence(bfdPacket *BfdControlPacket) error {
	if session.state.SessionState != STATE_ADMIN_DOWN {
		if bfdPacket.Poll {
			session.server.logger.Info(fmt.Sprintln("Received packet with poll bit for session ", session.state.SessionId))
			session.pollSequenceFinal = true
		}
		if bfdPacket.Final {
			session.server.logger.Info(fmt.Sprintln("Received packet with final bit for session ", session.state.SessionId))
			session.pollSequence = false
		}
		session.txTimer.Reset(0)
	}
	return nil
}
