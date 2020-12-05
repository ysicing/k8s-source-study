// MIT License
// Copyright (c) 2020 ysicing <i@ysicing.me>

package main

import (
	"fmt"
	"github.com/pkg/browser"
	"github.com/ysicing/ext/utils/exos"
	"github.com/ysicing/k8s-source-study/portforward"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
	"os/signal"
	"path/filepath"
)

func GetKubeConfigClient() (*rest.Config, *k8s.Clientset, error) {
	var kubeconfig string
	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	client, err := k8s.NewForConfig(config)
	if err != nil {
		return config, nil, err
	}
	return config, client, nil
}

func main() {
	config, _, err := GetKubeConfigClient()
	if err != nil {
		panic(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	portForward, err := portforward.NewPortForward(
		config,
		"monitoring",
		"prometheus-k8s",
		39999,
		9090,
		true)
	if err != nil {
		panic(err)
	}
	if err = portForward.Init(); err != nil {
		panic(err)
	}
	go func() {
		<-signals
		portForward.Stop()
	}()
	var webURL string = fmt.Sprintf("http://127.0.0.1:%d", 39999)
	log.Println(webURL)
	if exos.IsMacOS() {
		err = browser.OpenURL(webURL)
		if err != nil {
			panic(err)
		}
	}
	<-portForward.GetStop()
}
