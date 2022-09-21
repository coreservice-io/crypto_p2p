package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
)

var (
	cfg *config
)

func msndMain(serverChan chan<- *server) error {

	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg

	interrupt := interruptListener()
	defer fmt.Println("Shutdown complete")
	if interruptRequested(interrupt) {
		return nil
	}

	server, err := newServer(cfg.Listeners, interrupt)
	if err != nil {
		server.log.Errorln("Failed to start server on %v: %v", cfg.Listeners, err)
		return err
	}
	defer func() {
		server.log.Infoln("Gracefully shutting down the server...")
		server.Stop()
		server.WaitForShutdown()
	}()

	server.Start()
	if serverChan != nil {
		serverChan <- server
	}

	<-interrupt
	return nil
}

func interruptListener() <-chan struct{} {
	c := make(chan struct{})
	return c
}

func interruptRequested(interrupted <-chan struct{}) bool {
	select {
	case <-interrupted:
		return true
	default:
	}
	return false
}

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	debug.SetGCPercent(10)

	if err := msndMain(nil); err != nil {
		fmt.Printf("Failed to start msnd server: %v", err)
		os.Exit(1)
	}
}
