// MIT License
// Copyright (c) 2020 ysicing <i@ysicing.me>

package main

import (
	"github.com/ysicing/k8s-source-study/portforward"
	"log"
	"time"
)

func main()  {
	pf, err := portforward.NewPortForward("monitoring","prometheus-k8s", 39090, 9090)
	if err != nil {
		log.Fatal("Error setting up port forwarder: ", err)
	}
	err = pf.Start()
	if err != nil {
		log.Fatal("Error starting port forward: ", err)
	}

	log.Printf("Started tunnel on %d\n", pf.ListenPort)
	time.Sleep(60 * time.Second)
}