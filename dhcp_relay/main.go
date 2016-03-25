// Main entry point for DHCP_RELAY
package main

import (
	"flag"
	"fmt"
	"l3/dhcp_relay/server"
	"utils/logging"
)

const IP = "localhost"
const DHCP_RELAY_PORT = "9000"

func main() {
	fmt.Println("Starting dhcprelay daemon")
	paramsDir := flag.String("params", "./params", "Params directory")
	flag.Parse()
	fileName := *paramsDir
	if fileName[len(fileName)-1] != '/' {
		fileName = fileName + "/"
	}

	fmt.Println("Start logger")
	logger, err := logging.NewLogger(fileName, "dhcprelayd", "DRA")
	if err != nil {
		fmt.Println("Failed to start the logger. Exiting!!")
		return
	}
	go logger.ListenForSysdNotifications()
	logger.Info("Started the logger successfully.")

	var addr = IP + ":" + DHCP_RELAY_PORT
	fmt.Println("DHCP RELAY address is", addr)
	logger.Info(fmt.Sprintln("Starting DHCP RELAY...."))
	// Create a handler
	handler := relayServer.NewDhcpRelayServer()
	err = relayServer.StartServer(logger, handler, addr, fileName)
	if err != nil {
		logger.Err(fmt.Sprintln("DRA: Cannot start dhcp server", err))
		return
	}
}
