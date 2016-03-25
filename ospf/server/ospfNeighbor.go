package server

import (
	"encoding/binary"
	"fmt"
	"l3/ospf/config"
	"time"
)

/* @fn exchangePacketDiscardCheck
    Function to check SeqNumberMismatch
	for dbd exchange state packets.
*/
func (server *OSPFServer) exchangePacketDiscardCheck(nbrConf OspfNeighborEntry, nbrDbPkt ospfDatabaseDescriptionData) (isDiscard bool) {
	if nbrDbPkt.msbit != nbrConf.isMaster {
		server.logger.Info(fmt.Sprintln("NBREVENT: SeqNumberMismatch. Nbr should be master"))
		return true
	}

	if nbrDbPkt.ibit == true {
		server.logger.Info("NBREVENT:SeqNumberMismatch . Nbr ibit is true ")
		return true
	}
	/*
		if nbrDbPkt.options != INTF_OPTIONS {
			server.logger.Info(fmt.Sprintln("NBREVENT:SeqNumberMismatch. Nbr options dont match. Nbr options ", INTF_OPTIONS,
				" dbd oackts options", nbrDbPkt.options))
			return true
		}*/

	if nbrConf.isMaster {
		if nbrDbPkt.dd_sequence_number != nbrConf.ospfNbrSeqNum {
			if nbrDbPkt.dd_sequence_number+1 == nbrConf.ospfNbrSeqNum {
				server.logger.Info(fmt.Sprintln("Duplicate: This is db duplicate packet. Ignore."))
				return false
			}
			server.logger.Info(fmt.Sprintln("NBREVENT:SeqNumberMismatch : Nbr is master but dbd packet seq no doesnt match. dbd seq ",
				nbrDbPkt.dd_sequence_number, "nbr seq ", nbrConf.ospfNbrSeqNum))
			return true
		}
	} else {
		if nbrDbPkt.dd_sequence_number != nbrConf.ospfNbrSeqNum {
			server.logger.Info(fmt.Sprintln("NBREVENT:SeqNumberMismatch : Nbr is slave but dbd packet seq no doesnt match.dbd seq ",
				nbrDbPkt.dd_sequence_number, "nbr seq ", nbrConf.ospfNbrSeqNum))
			return true
		}
	}

	return false
}

func (server *OSPFServer) adjacancyEstablishementCheck(isNbrDRBDR bool, isRtrDRBDR bool) (result bool) {
	if isNbrDRBDR || isRtrDRBDR {
		return true
	}
	/* TODO - check if n/w is p2p , p2mp, virtual link */
	return false
}

func (server *OSPFServer) processDBDEvent(nbrKey NeighborConfKey, nbrDbPkt ospfDatabaseDescriptionData) {
	_, exists := server.NeighborConfigMap[nbrKey.OspfNbrRtrId]
	var dbd_mdata ospfDatabaseDescriptionData
	last_exchange := true
	if exists {
		nbrConf := server.NeighborConfigMap[nbrKey.OspfNbrRtrId]
		//intConf := server.IntfConfMap[nbrConf.intfConfKey]
		switch nbrConf.OspfNbrState {
		case config.NbrAttempt:
			/* reject packet */
			return
		case config.NbrInit:
		case config.NbrExchangeStart:
			//intfKey := nbrConf.intfConfKey
			var isAdjacent bool
			var negotiationDone bool
			isAdjacent = server.adjacancyEstablishementCheck(nbrConf.isDRBDR, true)
			if isAdjacent || nbrConf.OspfNbrState == config.NbrExchangeStart {
				// change nbr state
				nbrConf.OspfNbrState = config.NbrExchangeStart
				// decide master slave relation
				if nbrKey.OspfNbrRtrId > binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId) {
					nbrConf.isMaster = true
				} else {
					nbrConf.isMaster = false
				}
				/* The initialize(I), more (M) and master(MS) bits are set,
				   the contents of the packet are empty, and the neighbor's
				   Router ID is larger than the router's own.  In this case
				   the router is now Slave.  Set the master/slave bit to
				   slave, and set the neighbor data structure's DD sequence
				   number to that specified by the master.
				*/
				server.logger.Info(fmt.Sprintln("NBRDBD: nbr rtr id ", nbrKey.OspfNbrRtrId,
					" my router id ", binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId),
					" nbr_seq ", nbrConf.ospfNbrSeqNum, "dbd_seq no ", nbrDbPkt.dd_sequence_number))
				if nbrDbPkt.ibit && nbrDbPkt.mbit && nbrDbPkt.msbit &&
					nbrKey.OspfNbrRtrId > binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId) {
					server.logger.Info(fmt.Sprintln("DBD: (ExStart/slave) SLAVE = self,  MASTER = ", nbrKey.OspfNbrRtrId))
					nbrConf.isMaster = true
					server.logger.Info("NBREVENT: Negotiation done..")
					negotiationDone = true
					nbrConf.OspfNbrState = config.NbrExchange
					nbrConf.nbrEvent = config.NbrNegotiationDone
				}
				if nbrDbPkt.msbit && nbrKey.OspfNbrRtrId > binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId) {
					server.logger.Info(fmt.Sprintln("DBD: (ExStart/slave) SLAVE = self,  MASTER = ", nbrKey.OspfNbrRtrId))
					nbrConf.isMaster = true
					server.logger.Info("NBREVENT: Negotiation done..")
					negotiationDone = true
					nbrConf.OspfNbrState = config.NbrExchange
					nbrConf.nbrEvent = config.NbrNegotiationDone
				}

				/*   The initialize(I) and master(MS) bits are off, the
				     packet's DD sequence number equals the neighbor data
				     structure's DD sequence number (indicating
				     acknowledgment) and the neighbor's Router ID is smaller
				     than the router's own.  In this case the router is
				     Master. */
				if nbrDbPkt.ibit == false && nbrDbPkt.msbit == false &&
					nbrDbPkt.dd_sequence_number == nbrConf.ospfNbrSeqNum &&
					nbrKey.OspfNbrRtrId < binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId) {
					nbrConf.isMaster = false
					server.logger.Info(fmt.Sprintln("DBD:(ExStart) SLAVE = ", nbrKey.OspfNbrRtrId, "MASTER = SELF"))
					server.logger.Info("NBREVENT: Negotiation done..")
					negotiationDone = true
					nbrConf.OspfNbrState = config.NbrExchange
					nbrConf.nbrEvent = config.NbrNegotiationDone
				}

			} else {
				nbrConf.OspfNbrState = config.NbrTwoWay
			}

			var lsa_attach uint8
			if negotiationDone {
				server.logger.Info(fmt.Sprintln("DBD: (Exstart) lsa_headers = ", len(nbrDbPkt.lsa_headers)))
				server.generateDbSummaryList(nbrKey)
				if nbrConf.isMaster != true { // i am the master
					dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, false, true, true,
						nbrDbPkt.options, nbrDbPkt.dd_sequence_number+1, true, false)
				} else {
					// send acknowledgement DBD with I and MS bit false , mbit = 1
					dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, false, true, false,
						nbrDbPkt.options, nbrDbPkt.dd_sequence_number, true, false)
					dbd_mdata.dd_sequence_number++
				}

				if last_exchange {
					nbrConf.nbrEvent = config.NbrExchangeDone
				}

			} else { // negotiation not done
				nbrConf.OspfNbrState = config.NbrExchangeStart
				if nbrConf.isMaster &&
					nbrKey.OspfNbrRtrId > binary.BigEndian.Uint32(server.ospfGlobalConf.RouterId) {
					dbd_mdata.dd_sequence_number = nbrDbPkt.dd_sequence_number
					dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, true, true, true,
						nbrDbPkt.options, nbrDbPkt.dd_sequence_number, false, false)
					dbd_mdata.dd_sequence_number++
				} else {
					//start with new seq number
					dbd_mdata.dd_sequence_number = uint32(time.Now().Nanosecond()) //nbrConf.ospfNbrSeqNum
					dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, true, true, true,
						nbrDbPkt.options, nbrDbPkt.dd_sequence_number, false, false)
				}
			}

			nbrConfMsg := ospfNeighborConfMsg{
				ospfNbrConfKey: NeighborConfKey{
					OspfNbrRtrId: nbrKey.OspfNbrRtrId,
				},
				ospfNbrEntry: OspfNeighborEntry{
					OspfNbrIPAddr:          nbrConf.OspfNbrIPAddr,
					OspfRtrPrio:            nbrConf.OspfRtrPrio,
					intfConfKey:            nbrConf.intfConfKey,
					OspfNbrOptions:         0,
					OspfNbrState:           nbrConf.OspfNbrState,
					OspfNbrInactivityTimer: time.Now(),
					OspfNbrDeadTimer:       nbrConf.OspfNbrDeadTimer,
					ospfNbrSeqNum:          dbd_mdata.dd_sequence_number,
					isSeqNumUpdate:         true,
					isMaster:               nbrConf.isMaster,
					nbrEvent:               nbrConf.nbrEvent,
					ospfNbrLsaIndex:        nbrConf.ospfNbrLsaIndex + lsa_attach,
				},
				nbrMsgType: NBRUPD,
			}
			server.neighborConfCh <- nbrConfMsg
			OspfNeighborLastDbd[nbrKey] = dbd_mdata

		case config.NbrExchange:
			isDiscard := server.exchangePacketDiscardCheck(nbrConf, nbrDbPkt)
			if isDiscard {
				server.logger.Info(fmt.Sprintln("NBRDBD: Discard packet. nbr", nbrKey.OspfNbrRtrId,
					" nbr state ", nbrConf.OspfNbrState))

				nbrConf.OspfNbrState = config.NbrExchangeStart
				//invalidate all lists.
				newDbdMsg(nbrKey.OspfNbrRtrId, OspfNeighborLastDbd[nbrKey])
			} else { // process exchange state
				/* 2) Add lsa_headers to db packet from db_summary list */

				if nbrConf.isMaster != true { // i am master
					/* Send the DBD only if packet has mbit =1 or event != NbrExchangeDone
						send DBD with seq num + 1 , ibit = 0 ,  ms = 1
					 * if this is the last DBD for LSA description set mbit = 0
					*/
					server.logger.Info(fmt.Sprintln("DBD:(master/Exchange) nbr_event ", nbrConf.nbrEvent, " mbit ", nbrDbPkt.mbit))
					if nbrDbPkt.dd_sequence_number == nbrConf.ospfNbrSeqNum &&
						(nbrConf.nbrEvent != config.NbrExchangeDone ||
							nbrDbPkt.mbit) {
						server.logger.Info(fmt.Sprintln("DBD: (master/Exchange) Send next packet in the exchange  to nbr ", nbrKey.OspfNbrRtrId))
						dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, false, false, true,
							nbrDbPkt.options, nbrDbPkt.dd_sequence_number+1, true, false)
						OspfNeighborLastDbd[nbrKey] = dbd_mdata
					} /*else {
						// send old packet
						server.logger.Info(fmt.Sprintln("DBD: (master/exchange) Duplicated dbd. Resend . dbd_seq , nbr_seq_num ",
							nbrDbPkt.dd_sequence_number, nbrConf.ospfNbrSeqNum))
						data := newDbdMsg(nbrKey.OspfNbrRtrId, OspfNeighborLastDbd[nbrKey])
						server.ospfNbrDBDSendCh <- data
					}*/

					/* 1) get lsa headers update in req_list */
					headers_len := len(nbrDbPkt.lsa_headers)
					server.logger.Info(fmt.Sprintln("DBD: (Exchange) Received . nbr,total_lsa ", nbrKey.OspfNbrRtrId, headers_len))
					req_list := ospfNeighborRequest_list[nbrKey.OspfNbrRtrId]
					for i := 0; i < headers_len; i++ {
						var lsaheader ospfLSAHeader
						lsaheader = nbrDbPkt.lsa_headers[i]
						result := server.lsaAddCheck(lsaheader, nbrConf) // check lsdb
						if result {
							req := newospfNeighborReq()
							req.lsa_headers = lsaheader
							req.valid = true
							nbrConf.req_list_mutex.Lock()
							req_list = append(req_list, req)
							nbrConf.req_list_mutex.Unlock()
						}
					}
					ospfNeighborRequest_list[nbrKey.OspfNbrRtrId] = req_list
					server.logger.Info(fmt.Sprintln("DBD:(Exchange) Total elements in req_list ", len(ospfNeighborRequest_list[nbrKey.OspfNbrRtrId])))

				} else { // i am slave
					/* send acknowledgement DBD with I and MS bit false and mbit same as
					rx packet
					 if mbit is 0 && last_exchange == true generate NbrExchangeDone*/
					server.logger.Info(fmt.Sprintln("DBD: (slave/Exchange) Send next packet in the exchange  to nbr ", nbrKey.OspfNbrRtrId))
					if nbrDbPkt.dd_sequence_number == nbrConf.ospfNbrSeqNum {
						dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, false, nbrDbPkt.mbit, false,
							nbrDbPkt.options, nbrDbPkt.dd_sequence_number, true, false)
						OspfNeighborLastDbd[nbrKey] = dbd_mdata
						dbd_mdata.dd_sequence_number++
					} else {
						server.logger.Info(fmt.Sprintln("DBD: (slave/exchange) Duplicated dbd. Resend . dbd_seq , nbr_seq_num ",
							nbrDbPkt.dd_sequence_number, nbrConf.ospfNbrSeqNum))
						// send old ACK
						data := newDbdMsg(nbrKey.OspfNbrRtrId, OspfNeighborLastDbd[nbrKey])
						server.ospfNbrDBDSendCh <- data

						dbd_mdata = OspfNeighborLastDbd[nbrKey]

					}
					if !nbrDbPkt.mbit && last_exchange {
						nbrConf.nbrEvent = config.NbrExchangeDone
					}
				}
				if !nbrDbPkt.mbit || last_exchange {
					server.logger.Info(fmt.Sprintln("DBD: Exchange done with nbr ", nbrKey.OspfNbrRtrId))
					nbrConf.OspfNbrState = config.NbrLoading
					server.lsaReTxTimerCheck(nbrKey.OspfNbrRtrId)
				}
				if !nbrDbPkt.mbit && last_exchange {
					nbrConf.OspfNbrState = config.NbrLoading
					server.logger.Info(fmt.Sprintln("DBD: FULL , nbr ", nbrKey.OspfNbrRtrId))
					server.updateNeighborMdata(nbrConf.intfConfKey, nbrKey.OspfNbrRtrId)
					server.CreateNetworkLSACh <- ospfIntfToNbrMap[nbrConf.intfConfKey]
				}
			}

			nbrConfMsg := ospfNeighborConfMsg{
				ospfNbrConfKey: NeighborConfKey{
					OspfNbrRtrId: nbrKey.OspfNbrRtrId,
				},
				ospfNbrEntry: OspfNeighborEntry{
					OspfNbrIPAddr:          nbrConf.OspfNbrIPAddr,
					OspfRtrPrio:            nbrConf.OspfRtrPrio,
					intfConfKey:            nbrConf.intfConfKey,
					OspfNbrOptions:         0,
					OspfNbrState:           nbrConf.OspfNbrState,
					OspfNbrInactivityTimer: time.Now(),
					OspfNbrDeadTimer:       nbrConf.OspfNbrDeadTimer,
					ospfNbrSeqNum:          dbd_mdata.dd_sequence_number,
					isSeqNumUpdate:         true,
					isMaster:               nbrConf.isMaster,
					nbrEvent:               nbrConf.nbrEvent,
					ospfNbrLsaReqIndex:     nbrConf.ospfNbrLsaReqIndex,
				},
				nbrMsgType: NBRUPD,
			}
			server.neighborConfCh <- nbrConfMsg

		case config.NbrLoading, config.NbrFull:

			var seq_num uint32
			server.logger.Info(fmt.Sprintln("DBD: Loading . Nbr ", nbrKey.OspfNbrRtrId))
			isDiscard := server.exchangePacketDiscardCheck(nbrConf, nbrDbPkt)
			if isDiscard {
				server.logger.Info(fmt.Sprintln("NBRDBD:Loading  Discard packet. nbr", nbrKey.OspfNbrRtrId,
					" nbr state ", nbrConf.OspfNbrState))
				//update neighbor to exchange start state and send dbd

				nbrConf.OspfNbrState = config.NbrExchangeStart
				nbrConf.nbrEvent = config.Nbr2WayReceived
				nbrConf.isMaster = false
				dbd_mdata, last_exchange = server.ConstructAndSendDbdPacket(nbrKey, true, true, true,
					nbrDbPkt.options, nbrConf.ospfNbrSeqNum+1, false, false)
				seq_num = dbd_mdata.dd_sequence_number
			} else {
				/* dbd received in this stage is duplicate.
				    slave - Send the old dbd packet.
					master - discard
				*/
				if nbrConf.isMaster {
					dbd_mdata, _ := server.ConstructAndSendDbdPacket(nbrKey, false, nbrDbPkt.mbit, false,
						nbrDbPkt.options, nbrConf.ospfNbrSeqNum, false, false)
					seq_num = dbd_mdata.dd_sequence_number + 1
				}
				nbrConf.ospfNbrLsaReqIndex = server.BuildAndSendLSAReq(nbrKey.OspfNbrRtrId, nbrConf)
				seq_num = OspfNeighborLastDbd[nbrKey].dd_sequence_number
				nbrConf.OspfNbrState = config.NbrFull
			}

			nbrConfMsg := ospfNeighborConfMsg{
				ospfNbrConfKey: NeighborConfKey{
					OspfNbrRtrId: nbrKey.OspfNbrRtrId,
				},
				ospfNbrEntry: OspfNeighborEntry{
					OspfNbrIPAddr:          nbrConf.OspfNbrIPAddr,
					OspfRtrPrio:            nbrConf.OspfRtrPrio,
					intfConfKey:            nbrConf.intfConfKey,
					OspfNbrOptions:         0,
					OspfNbrState:           nbrConf.OspfNbrState,
					OspfNbrInactivityTimer: time.Now(),
					OspfNbrDeadTimer:       nbrConf.OspfNbrDeadTimer,
					ospfNbrSeqNum:          seq_num,
					isSeqNumUpdate:         true,
					isMaster:               nbrConf.isMaster,
					ospfNbrLsaReqIndex:     nbrConf.ospfNbrLsaReqIndex,
				},
				nbrMsgType: NBRUPD,
			}
			server.neighborConfCh <- nbrConfMsg
			server.updateNeighborMdata(nbrConf.intfConfKey, nbrKey.OspfNbrRtrId)
			server.logger.Info(fmt.Sprintln("NBREVENT: Flood the LSA. nbr full state ", nbrKey.OspfNbrRtrId))
			server.CreateNetworkLSACh <- ospfIntfToNbrMap[nbrConf.intfConfKey]
		case config.NbrTwoWay:
			/* ignore packet */
			server.logger.Info(fmt.Sprintln("NBRDBD: Ignore packet as NBR state is two way"))
			return
		case config.NbrDown:
			/* ignore packet. */
			server.logger.Info(fmt.Sprintln("NBRDBD: Ignore packet . NBR is down"))
			return
		} // end of switch
	} else { //nbr doesnt exist
		server.logger.Info(fmt.Sprintln("Ignore DB packet. Nbr doesnt exist ", nbrKey))
		return
	}
}

func (server *OSPFServer) ProcessNbrStateMachine() {
	for {

		select {
		case nbrData := <-(server.neighborHelloEventCh):
			server.logger.Info(fmt.Sprintln("NBREVENT: Received hellopkt event for nbrId ", nbrData.RouterId, " two_way", nbrData.TwoWayStatus))
			var nbrConf OspfNeighborEntry
			var send_dbd bool
			var seq_update bool
			var dbd_mdata ospfDatabaseDescriptionData

			//Check if neighbor exists
			_, exists := server.NeighborConfigMap[nbrData.RouterId]
			send_dbd = false
			seq_update = false
			if exists {
				nbrConf = server.NeighborConfigMap[nbrData.RouterId]
				if nbrData.TwoWayStatus { // update the state
					startAdjacency := server.adjacancyEstablishementCheck(nbrConf.isDRBDR, true)
					if startAdjacency && nbrConf.OspfNbrState == config.NbrTwoWay {
						nbrConf.OspfNbrState = config.NbrExchangeStart
						if nbrConf.ospfNbrSeqNum == 0 {
							nbrConf.ospfNbrSeqNum = uint32(time.Now().Unix())
							dbd_mdata.dd_sequence_number = nbrConf.ospfNbrSeqNum
							dbd_mdata.msbit = true // i am master
							dbd_mdata.ibit = true
							dbd_mdata.mbit = true
							nbrConf.isMaster = false
							seq_update = true
						} else {
							dbd_mdata.dd_sequence_number = nbrConf.ospfNbrSeqNum
							dbd_mdata.msbit = true
							nbrConf.isMaster = false
						}
						dbd_mdata.interface_mtu = INTF_MTU_MIN
						server.logger.Info(fmt.Sprintln("NBRHELLO: Send, seq no ", dbd_mdata.dd_sequence_number,
							"msbit ", dbd_mdata.msbit))
						send_dbd = true
					} else { // no adjacency
						if nbrConf.OspfNbrState < config.NbrTwoWay {
							nbrConf.OspfNbrState = config.NbrTwoWay
						}
					}
				} else {
					nbrConf.OspfNbrState = config.NbrInit
				}

				nbrConfMsg := ospfNeighborConfMsg{
					ospfNbrConfKey: NeighborConfKey{
						OspfNbrRtrId: nbrData.RouterId,
					},
					ospfNbrEntry: OspfNeighborEntry{
						OspfNbrIPAddr:          nbrConf.OspfNbrIPAddr,
						OspfRtrPrio:            nbrConf.OspfRtrPrio,
						intfConfKey:            nbrConf.intfConfKey,
						OspfNbrOptions:         0,
						OspfNbrState:           nbrConf.OspfNbrState,
						OspfNbrInactivityTimer: time.Now(),
						OspfNbrDeadTimer:       nbrConf.OspfNbrDeadTimer,
						ospfNbrDBDTickerCh:     nbrConf.ospfNbrDBDTickerCh,
						ospfNbrSeqNum:          nbrConf.ospfNbrSeqNum,
						isSeqNumUpdate:         seq_update,
						isMaster:               nbrConf.isMaster,
						nbrEvent:               nbrConf.nbrEvent,
					},
					nbrMsgType: NBRUPD,
				}
				server.neighborConfCh <- nbrConfMsg

				if send_dbd {
					server.ConstructAndSendDbdPacket(nbrConfMsg.ospfNbrConfKey, true, true, true,
						INTF_OPTIONS, nbrConf.ospfNbrSeqNum, false, false)
				}
				server.logger.Info(fmt.Sprintln("NBREVENT: update Nbr ", nbrData.RouterId, "state ", nbrConf.OspfNbrState))

			} else { //neighbor doesnt exist
				var ticker *time.Ticker
				var nbrState config.NbrState
				var dbd_mdata ospfDatabaseDescriptionData
				var send_dbd bool
				server.logger.Info(fmt.Sprintln("NBREVENT: Create new neighbor with id ", nbrData.RouterId))

				if nbrData.TwoWayStatus { // update the state
					startAdjacency := server.adjacancyEstablishementCheck(false, true)
					if startAdjacency {
						nbrState = config.NbrExchangeStart
						dbd_mdata.dd_sequence_number = uint32(time.Now().Nanosecond())
						seq_update = true
						// send dbd packets
						ticker = time.NewTicker(time.Second * 10)
						send_dbd = true
						server.logger.Info(fmt.Sprintln("NBRHELLO: Send, seq no ", dbd_mdata.dd_sequence_number,
							"msbit ", dbd_mdata.msbit))
					} else { // no adjacency
						nbrState = config.NbrTwoWay
						send_dbd = false
					}
				} else {
					nbrState = config.NbrInit
					send_dbd = false
				}

				nbrConfMsg := ospfNeighborConfMsg{
					ospfNbrConfKey: NeighborConfKey{
						OspfNbrRtrId: nbrData.RouterId,
					},
					ospfNbrEntry: OspfNeighborEntry{
						OspfNbrIPAddr:          nbrData.NeighborIP,
						OspfRtrPrio:            nbrData.RtrPrio,
						intfConfKey:            nbrData.IntfConfKey,
						OspfNbrOptions:         0,
						OspfNbrState:           nbrState,
						OspfNbrInactivityTimer: time.Now(),
						OspfNbrDeadTimer:       nbrData.nbrDeadTimer,
						ospfNbrSeqNum:          dbd_mdata.dd_sequence_number,
						isSeqNumUpdate:         seq_update,
						ospfNbrDBDTickerCh:     ticker,
					},
					nbrMsgType: NBRADD,
				}
				/* add the stub entry so that till the update thread updates the data
				valid entry will be present in the map */
				server.NeighborConfigMap[nbrData.RouterId] = nbrConfMsg.ospfNbrEntry
				server.initNeighborMdata(nbrData.IntfConfKey)
				server.neighborConfCh <- nbrConfMsg
				if send_dbd {
					dbd_mdata.ibit = true
					dbd_mdata.mbit = true
					dbd_mdata.msbit = true

					dbd_mdata.interface_mtu = INTF_MTU_MIN
					dbd_mdata.options = INTF_OPTIONS
				}
				server.logger.Info(fmt.Sprintln("NBREVENT: ADD Nbr ", nbrData.RouterId, "state ", nbrState))
			}

		case nbrDbPkt := <-(server.neighborDBDEventCh):
			server.logger.Info(fmt.Sprintln("NBREVENT: DBD received  ", nbrDbPkt))
			server.processDBDEvent(nbrDbPkt.ospfNbrConfKey, nbrDbPkt.ospfNbrDBDData)

		case state := <-server.neighborFSMCtrlCh:
			if state == false {
				return
			}
		}
	} // end of for
}

func (server *OSPFServer) ProcessRxNbrPkt() {
	for {
		select {
		case nbrLSAReqPkt := <-(server.neighborLSAReqEventCh):
			nbr, exists := server.NeighborConfigMap[nbrLSAReqPkt.nbrKey]
			if exists && nbr.OspfNbrState >= config.NbrExchange {
				server.DecodeLSAReq(nbrLSAReqPkt)
			}

		case nbrLSAUpdPkt := <-(server.neighborLSAUpdEventCh):
			nbr, exists := server.NeighborConfigMap[nbrLSAUpdPkt.nbrKey]

			if exists && nbr.OspfNbrState >= config.NbrExchange {
				server.DecodeLSAUpd(nbrLSAUpdPkt)
			}

		case nbrLSAAckPkt := <-(server.neighborLSAACKEventCh):
			nbr, exists := server.NeighborConfigMap[nbrLSAAckPkt.nbrKey]

			if exists && nbr.OspfNbrState >= config.NbrExchange {
				server.logger.Info(fmt.Sprintln("ACK : received - ", nbrLSAAckPkt))
				//server.DecodeLSAAck(nbrLSAAckPkt)
			}

		case stop := <-(server.ospfRxNbrPktStopCh):
			if stop {
				return
			}

		}

	}
}

func (server *OSPFServer) ProcessTxNbrPkt() {
	for {
		select {
		case dbd_mdata := <-server.ospfNbrDBDSendCh:
			nbrConf, exists := server.NeighborConfigMap[dbd_mdata.ospfNbrConfKey.OspfNbrRtrId]
			if exists {
				intConf, exist := server.IntfConfMap[nbrConf.intfConfKey]
				if exist {
					dstMac, _ := ospfNeighborIPToMAC[dbd_mdata.ospfNbrConfKey.OspfNbrRtrId]
					data := server.BuildDBDPkt(nbrConf.intfConfKey, intConf, nbrConf,
						dbd_mdata.ospfNbrDBDData, dstMac)
					server.SendOspfPkt(nbrConf.intfConfKey, data)
				}

			}

		case lsa_data := <-server.ospfNbrLsaReqSendCh:
			nbrConf, exists := server.NeighborConfigMap[lsa_data.nbrKey]
			if exists {
				intConf, exist := server.IntfConfMap[nbrConf.intfConfKey]
				if exist {
					dstMac, _ := ospfNeighborIPToMAC[lsa_data.nbrKey]
					server.logger.Info(fmt.Sprintln("Send LSA: nbrconf ,  lsa_data", nbrConf, lsa_data))
					data := server.EncodeLSAReqPkt(nbrConf.intfConfKey, intConf, nbrConf, lsa_data.lsa_slice, dstMac)
					server.SendOspfPkt(nbrConf.intfConfKey, data)
				}

			}

		case msg := <-server.ospfNbrLsaUpdSendCh:
			server.processTxLsaUpdate(msg)

		case msg := <-server.ospfNbrLsaAckSendCh:
			server.processTxLsaAck(msg)

		case stop := <-server.ospfTxNbrPktStopCh:
			if stop == true {
				return
			}
		}
	}

}

func (server *OSPFServer) generateDbSummaryList(nbrConfKey NeighborConfKey) {
	nbrConf, exists := server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId]

	if !exists {
		server.logger.Err(fmt.Sprintln("negotiation: db_list Nbr  doesnt exist. nbr ", nbrConfKey))
		return
	}
	intf, _ := server.IntfConfMap[nbrConf.intfConfKey]
	nbrMdata, exists := ospfIntfToNbrMap[nbrConf.intfConfKey]

	areaId := convertIPv4ToUint32(intf.IfAreaId)
	lsdbKey := LsdbKey{
		AreaId: areaId,
	}
	area_lsa, exist := server.AreaLsdb[lsdbKey]
	if !exist {
		server.logger.Err(fmt.Sprintln("negotiation: db_list self originated lsas dont exist. Nbr , lsdb_key ", nbrConfKey, lsdbKey))
		return
	}
	router_lsdb := area_lsa.RouterLsaMap
	network_lsa := area_lsa.NetworkLsaMap

	ospfNeighborDBSummary_list[nbrConfKey.OspfNbrRtrId] = nil
	db_list := []*ospfNeighborDBSummary{}
	for lsaKey, _ := range router_lsdb {
		// check if lsa instance is marked true
		db_summary := newospfNeighborDBSummary()
		drlsa, ret := server.getRouterLsaFromLsdb(areaId, lsaKey)
		if ret == LsdbEntryNotFound {
			continue
		}
		db_summary.lsa_headers = getLsaHeaderFromLsa(drlsa.LsaMd.LSAge, drlsa.LsaMd.Options,
			RouterLSA, lsaKey.LSId, lsaKey.AdvRouter,
			uint32(drlsa.LsaMd.LSSequenceNum), drlsa.LsaMd.LSChecksum,
			drlsa.LsaMd.LSLen)
		db_summary.valid = true
		/* add entry to the db summary list  */
		db_list = append(db_list, db_summary)
		lsid := convertUint32ToIPv4(lsaKey.LSId)
		server.logger.Info(fmt.Sprintln("negotiation: db_list append router lsid  ", lsid))
	} // end of for

	for networkKey, _ := range network_lsa {
		// check if lsa instance is marked true
		db_summary := newospfNeighborDBSummary()
		if nbrMdata.isDR {
			dnlsa, ret := server.getNetworkLsaFromLsdb(areaId, networkKey)
			if ret == LsdbEntryNotFound {
				continue
			}
			db_summary.lsa_headers = getLsaHeaderFromLsa(dnlsa.LsaMd.LSAge, dnlsa.LsaMd.Options,
				NetworkLSA, networkKey.LSId, intf.IfDRtrId,
				uint32(dnlsa.LsaMd.LSSequenceNum), dnlsa.LsaMd.LSChecksum,
				dnlsa.LsaMd.LSLen)
			db_summary.valid = true
			/* add entry to the db summary list  */
			db_list = append(db_list, db_summary)
			lsid := convertUint32ToIPv4(networkKey.LSId)
			server.logger.Info(fmt.Sprintln("negotiation: db_list append network lsid  ", lsid))
		} // end of for
	}

	for lsa := range db_list {
		rtr_id := convertUint32ToIPv4(db_list[lsa].lsa_headers.adv_router_id)
		server.logger.Info(fmt.Sprintln(lsa, ": ", rtr_id, " lsatype ", db_list[lsa].lsa_headers.ls_type))
	}
	nbrConf.db_summary_list_mutex.Lock()
	ospfNeighborDBSummary_list[nbrConfKey.OspfNbrRtrId] = db_list
	nbrConf.db_summary_list_mutex.Unlock()
}

func (server *OSPFServer) neighborDeadTimerEvent(nbrConfKey NeighborConfKey) {
	var nbr_entry_dead_func func()

	nbr_entry_dead_func = func() {
		server.logger.Info(fmt.Sprintln("NBRSCAN: DEAD ", nbrConfKey.OspfNbrRtrId))
		nbrStateChangeData := NbrStateChangeMsg{
			RouterId: nbrConfKey.OspfNbrRtrId,
		}

		_, exists := server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId]
		if exists {
			nbrConf := server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId]

			nbrConfMsg := ospfNeighborConfMsg{
				ospfNbrConfKey: NeighborConfKey{
					OspfNbrRtrId: nbrConfKey.OspfNbrRtrId,
				},
				ospfNbrEntry: OspfNeighborEntry{
					OspfNbrIPAddr:          nbrConf.OspfNbrIPAddr,
					OspfRtrPrio:            nbrConf.OspfRtrPrio,
					intfConfKey:            nbrConf.intfConfKey,
					OspfNbrOptions:         0,
					OspfNbrState:           config.NbrDown,
					OspfNbrInactivityTimer: time.Now(),
					OspfNbrDeadTimer:       nbrConf.OspfNbrDeadTimer,
				},
				nbrMsgType: NBRDEL,
			}
			// update neighbor map
			server.neighborConfCh <- nbrConfMsg
			intfConf := server.IntfConfMap[nbrConf.intfConfKey]
			intfConf.NbrStateChangeCh <- nbrStateChangeData
		}
	} // end of afterFunc callback

	_, exists := server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId]
	if exists {
		nbrConf := server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId]
		nbrConf.NbrDeadTimer = time.AfterFunc(nbrConf.OspfNbrDeadTimer, nbr_entry_dead_func)
		server.NeighborConfigMap[nbrConfKey.OspfNbrRtrId] = nbrConf
	}

}

func (server *OSPFServer) refreshNeighborSlice() {
	go func() {
		for t := range server.neighborSliceRefCh.C {

			server.neighborBulkSlice = []uint32{}
			idx := 0
			for nbrKey, _ := range server.NeighborConfigMap {
				server.neighborBulkSlice = append(server.neighborBulkSlice, nbrKey)
				idx++
			}

			server.logger.Info(fmt.Sprintln("Get bulk slice refreshed - ", t))
		}
	}()

}
