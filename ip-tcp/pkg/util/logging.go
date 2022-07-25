package util

import (
	"io/ioutil"
	"log"
	"os"
)

// Debug is optional logger for debugging.
var Debug *log.Logger

// SetDebug turns debug print statements on or off.
func InitDebug(enabled bool) {
	Debug = log.New(ioutil.Discard, "DEBUG: ", log.Ltime|log.Lshortfile)
	if enabled {
		Debug.SetOutput(os.Stdout)
	} else {
		Debug.SetOutput(ioutil.Discard)
	}
}
