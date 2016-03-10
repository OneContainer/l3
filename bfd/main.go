package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"l3/bfd/rpc"
	"l3/bfd/server"
	"utils/logging"
)

func main() {
	fmt.Println("Starting bfd daemon")
	paramsDir := flag.String("params", "./params", "Params directory")
	flag.Parse()

	dbName := *paramsDir + "/UsrConfDb.db"
	fmt.Println("BFDd opening Config DB: ", dbName)
	dbHdl, err := sql.Open("sqlite3", dbName)
	if err != nil {
		fmt.Println("Failed to open connection to DB. ", err, " Exiting!!")
		return
	}

	fmt.Println("Start logger")
	logger, err := logging.NewLogger(*paramsDir, "bfdd", "BFD", dbHdl)
	if err != nil {
		fmt.Println("Failed to start the logger. Exiting!!")
		return
	}
	go logger.ListenForSysdNotifications()

	logger.Info("Started the logger successfully.")

	fileName := *paramsDir
	if fileName[len(fileName)-1] != '/' {
		fileName = fileName + "/"
	}
	fileName = fileName + "clients.json"

	logger.Info(fmt.Sprintln("Starting BFD Server..."))
	bfdServer := server.NewBFDServer(logger)
	go bfdServer.StartServer(fileName, dbHdl)
	logger.Info(fmt.Sprintln("Waiting for BFD server to come up"))
	up := <-bfdServer.ServerUpCh
	dbHdl.Close()
	logger.Info(fmt.Sprintln("BFD server is up: ", up))
	if !up {
		logger.Err(fmt.Sprintln("Exiting!!"))
		return
	}

	logger.Info(fmt.Sprintln("Starting Config listener..."))
	confIface := rpc.NewBFDHandler(logger, bfdServer)
	rpc.StartServer(logger, confIface, fileName)
}
