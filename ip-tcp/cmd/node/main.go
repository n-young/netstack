package main

import (
	"flag"
	"log"

	data "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/data"
	ip "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/ip"
	tcp "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/tcp"
	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// main parses args and runs the node.
func main() {
	// initialize logger
	log.SetFlags(0)
	// Set up CLI flags.
	var aggFlag bool
	flag.BoolVar(&aggFlag, "agg", false, "Turn on route aggregation.")
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Turn on debug message printing.")
	flag.BoolVar(&debug, "d", false, "Turn on debug message printing.")
	flag.Parse()
	// Enable Debugging mode
	util.InitDebug(debug)
	// node <linkfile>
	args := flag.Args()
	if len(args) < 1 {
		log.Println("usage: ./node <linkfile>")
		return
	}
	// Parse the linkfile.
	linkfile := args[0]
	node, err := ip.NewNode(linkfile)
	if err != nil {
		log.Println("Error reading lnx file, exiting")
		return
	}
	// Set Route Aggregation.
	node.SetAggregate(aggFlag)
	// Register protocol handlers.
	node.RegisterHandler(0, data.DataHandler)
	driver := tcp.InitDriver(node)
	node.RegisterHandler(6, driver.TCPHandler)
	// Run the server
	node.Run(false)
	driver.Run()
}
