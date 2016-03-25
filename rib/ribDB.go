// ribDB.go
package main

import (
	"fmt"
	"ribd"
	//"utils/commonDefs"
	//    "utils/dbutils"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func UpdateRoutesFromDB() (err error) {
	logger.Info(fmt.Sprintln("UpdateRoutesFromDB"))
	dbHdl := routeServiceHandler.DbHdl
	defer dbHdl.Close()
	dbCmd := "select * from IPv4Route"
	rows, err := dbHdl.Query(dbCmd)
	if err != nil {
		logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmd, err))
		return err
	}
	var ipRoute IPRoute
	for rows.Next() {
		if err = rows.Scan(&ipRoute.DestinationNw, &ipRoute.NetworkMask, &ipRoute.NextHopIp, &ipRoute.Cost, &ipRoute.OutgoingIntfType, &ipRoute.OutgoingInterface, &ipRoute.Protocol); err != nil {
			logger.Info(fmt.Sprintf("DB Scan failed when iterating over IPV4Route rows with error %s\n", err))
			return err
		}
        cfg := ribd.IPv4Route {ipRoute.OutgoingIntfType, ipRoute.Protocol, ipRoute.OutgoingInterface,ipRoute.DestinationNw,int32(ipRoute.Cost),ipRoute.NetworkMask,ipRoute.NextHopIp}
		_, err = routeServiceHandler.ProcessRouteCreateConfig(&cfg)//ipRoute.DestinationNw, ipRoute.NetworkMask, ribd.Int(ipRoute.Cost), ipRoute.NextHopIp, outIntfType, ribd.Int(outIntf), ipRoute.Protocol)
		//_,err = createV4Route(ipRoute.DestinationNw, ipRoute.NetworkMask, ribd.Int(ipRoute.Cost), ipRoute.NextHopIp, outIntfType,ribd.Int(outIntf), ribd.Int(proto),  FIBAndRIB,ribdCommonDefs.RoutePolicyStateChangetoValid,ribd.Int(len(destNetSlice)))
		if err != nil {
			logger.Info(fmt.Sprintf("Route create failed with err %s\n", err))
			return err
		}
	}
	return err
}

func UpdatePolicyConditionsFromDB(dbHdl *sql.DB) (err error) {
	logger.Info(fmt.Sprintln("UpdatePolicyConditionsFromDB"))
	dbCmd := "select * from PolicyCondition"
	rows, err := dbHdl.Query(dbCmd)
	if err != nil {
		logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmd, err))
		return err
	}
	var condition ribd.PolicyCondition
	var IpPrefix, MaskLengthRange string
	for rows.Next() {
		if err = rows.Scan(&condition.Name, &condition.ConditionType, &condition.MatchProtocol, &IpPrefix, &MaskLengthRange); err != nil {
			logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyDefinitionStmtMatchProtocolCondition rows with error %s\n", err))
			return err
		}
		//var cfgIpPrefix ribd.PolicyPrefix
		//var dstIpMatchPrefixconditionCfg ribd.PolicyDstIpMatchPrefixSetCondition
		//cfgIpPrefix.IpPrefix = IpPrefix
		//cfgIpPrefix.MasklengthRange = MaskLengthRange
		//dstIpMatchPrefixconditionCfg.Prefix = &cfgIpPrefix
		//condition.MatchDstIpPrefixConditionInfo = &dstIpMatchPrefixconditionCfg
		condition.IpPrefix = IpPrefix
		condition.MaskLengthRange = MaskLengthRange
		routeServiceHandler.ProcessPolicyConditionConfigCreate(&condition)
		if err != nil {
			logger.Info(fmt.Sprintf("Condition create failed with err %s\n", err))
			return err
		}
	}
	return err
}
func UpdatePolicyActionsFromDB(dbHdl *sql.DB) (err error) {
	logger.Info(fmt.Sprintln("UpdatePolicyActionsFromDB"))
	dbCmd := "select * from PolicyAction"
	rows, err := dbHdl.Query(dbCmd)
	if err != nil {
		logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmd, err))
		return err
	}
	var action ribd.PolicyAction
	for rows.Next() {
		if err = rows.Scan(&action.Name, &action.ActionType, &action.SetAdminDistanceValue, &action.Accept, &action.Reject, &action.RedistributeAction, &action.RedistributeTargetProtocol, &action.NetworkStatementTargetProtocol); err != nil {
			logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyDefinitionStmtMatchProtocolCondition rows with error %s\n", err))
			return err
		}
		_, err = routeServiceHandler.ProcessPolicyActionConfigCreate(&action)
		if err != nil {
			logger.Info(fmt.Sprintf("Action create failed with err %s\n", err))
			return err
		}
	}
	return err
}
func UpdatePolicyStmtsFromDB(dbHdl *sql.DB) (err error) {
	logger.Info(fmt.Sprintln("UpdatePolicyStmtsFromDB"))
	dbCmd := "select * from PolicyStmt"
	rows, err := dbHdl.Query(dbCmd)
	if err != nil {
		logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmd, err))
		return err
	}
	var stmt ribd.PolicyStmt
	for rows.Next() {
		if err = rows.Scan(&stmt.Name, &stmt.MatchConditions); err != nil {
			logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyDefinitionStmtMatchProtocolCondition rows with error %s\n", err))
			return err
		}
		logger.Info(fmt.Sprintln("Scanning stmt ", stmt.Name, "MatchConditions:",stmt.MatchConditions))
		dbCmdCond := "select * from PolicyStmtConditions"
		conditionrows, err := dbHdl.Query(dbCmdCond)
		if err != nil {
			logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmdCond, err))
			return err
		}
		stmt.Conditions = make([]string, 0)
		var Conditions, stmtName string
		for conditionrows.Next() {
			if err = conditionrows.Scan(&stmtName, &Conditions); err != nil {
				logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyStmtConfigConditions rows with error %s\n", err))
				return err
			}
			if stmtName != stmt.Name {
				logger.Info(fmt.Sprintln("Not a condition for this statement"))
				continue
			}
			logger.Info(fmt.Sprintln("Fetching condition ", Conditions))
			stmt.Conditions = append(stmt.Conditions, Conditions)
		}

		dbCmdAction := "select * from PolicyStmtActions"
		actionrows, err := dbHdl.Query(dbCmdAction)
		if err != nil {
			logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmdAction, err))
			return err
		}
		stmt.Actions = make([]string, 0)
		var Actions string
		for actionrows.Next() {
			if err = actionrows.Scan(&stmtName, &Actions); err != nil {
				logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyStmtConfigActions rows with error %s\n", err))
				return err
			}
			if stmtName != stmt.Name {
				logger.Info(fmt.Sprintln("Not a action for this statement"))
				continue
			}
			logger.Info(fmt.Sprintln("Fetching action ", Actions))
			stmt.Actions = append(stmt.Actions, Actions)
		}
		_, err = routeServiceHandler.ProcessPolicyStmtConfigCreate(&stmt)
		if err != nil {
			logger.Info(fmt.Sprintf("Action create failed with err %s\n", err))
			return err
		}
	}
	return err
}
func UpdatePolicyFromDB(dbHdl *sql.DB) (err error) {
	logger.Info(fmt.Sprintln("UpdatePolicyFromDB"))
	dbCmd := "select * from PolicyDefinition"
	rows, err := dbHdl.Query(dbCmd)
	if err != nil {
		logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmd, err))
		return err
	}
	var policy ribd.PolicyDefinition
	for rows.Next() {
		if err = rows.Scan(&policy.Name, &policy.Precedence, &policy.MatchType); err != nil {
			logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyDefinitionConfig rows with error %s\n", err))
			return err
		}
		logger.Info(fmt.Sprintln("executed cmd ", dbCmd, "policy name = ", policy.Name, " precedence: ", policy.Precedence))
		dbCmdPrecedence := "select * from PolicyDefinitionStatementList"
		conditionrows, err := dbHdl.Query(dbCmdPrecedence)
		if err != nil {
			logger.Info(fmt.Sprintf("DB Query failed for %s with err %s\n", dbCmdPrecedence, err))
			return err
		}
		policy.StatementList = make([]*ribd.PolicyDefinitionStmtPrecedence, 0)
		var stmt, policyName string
		var precedence int32
		for conditionrows.Next() {
			if err = conditionrows.Scan(&policyName, &precedence, &stmt); err != nil {
				logger.Info(fmt.Sprintf("DB Scan failed when iterating over PolicyDefinitionConfigStatementList rows with error %s\n", err))
				return err
			}
			if policyName != policy.Name {
				logger.Info(fmt.Sprintln("Not a stmt for this policy, policyName: ", policyName))
				continue
			}
			logger.Info(fmt.Sprintln("Fetching stmt ", stmt,"Precedence:",precedence))
			policyStmtPrecedence := ribd.PolicyDefinitionStmtPrecedence{Precedence: int32(precedence), Statement: stmt}
			policy.StatementList = append(policy.StatementList, &policyStmtPrecedence)
		}

		_, err = routeServiceHandler.ProcessPolicyDefinitionConfigCreate(&policy)
		if err != nil {
			logger.Info(fmt.Sprintf("policy create failed with err %s\n", err))
			return err
		}
	}
	return err
}
func UpdatePolicyObjectsFromDB() { //(paramsDir string) (err error) {
	logger.Info(fmt.Sprintln("UpdateFromDB"))
	dbHdl := routeServiceHandler.DbHdl
	UpdatePolicyConditionsFromDB(dbHdl) //paramsDir, dbHdl)
	UpdatePolicyActionsFromDB(dbHdl)    //paramsDir, dbHdl)
	UpdatePolicyStmtsFromDB(dbHdl)
	UpdatePolicyFromDB(dbHdl)
	return
}
