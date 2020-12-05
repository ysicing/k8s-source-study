// MIT License
// Copyright (c) 2020 ysicing <i@ysicing.me>

package portforward

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"net"
	"net/http"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

)

type PortForward struct {
	Config *rest.Config
	Clientset kubernetes.Interface
	DestinationPort int
	ListenPort int
	Namespace string
	ServiceName string
	stopChan  chan struct{}
	readyChan chan struct{}
}

func NewPortForward(namespace string, servicename string,sport, dport int) (*PortForward, error)  {
	pf := &PortForward{
		DestinationPort: dport,
		ListenPort: sport,
		Namespace:       namespace,
		ServiceName: servicename,
	}
	var err error
	pf.Config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
		).ClientConfig()
	if err != nil {
		return pf, errors.Wrap(err, "load kubecfg err")
	}
	pf.Clientset, err = kubernetes.NewForConfig(pf.Config)
	if err != nil {
		return pf, errors.Wrap(err, "create k8s client err")
	}
	return pf, nil
}

func (p *PortForward) getListenPort() (int, error) {
	var err error
	if p.ListenPort == 0 {
		p.ListenPort, err = p.getFreePort()
	}
	return p.ListenPort, err
}

func (p *PortForward) getFreePort() (int, error)  {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	port := listener.Addr().(*net.TCPAddr).Port
	err = listener.Close()
	if err != nil {
		return 0, err
	}
	return port, nil
}

// Create an httpstream.Dialer for use with portforward.New
func (p *PortForward) dialer() (httpstream.Dialer, error) {
	pod, err := p.getPodName()
	url := p.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(p.Namespace).
		Name(pod).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(p.Config)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create round tripper")
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	return dialer, nil
}

func (p *PortForward) Start() error {
	p.stopChan = make(chan  struct{}, 1)
	readyChan := make(chan struct{}, 1)
	errChan := make(chan error, 1)

	listenPort, err := p.getListenPort()
	if err != nil {
		return errors.Wrap(err, "Could not find a port to bind to")
	}

	dialer, err := p.dialer()
	if err != nil {
		return errors.Wrap(err, "Could not create a dialer")
	}

	ports := []string{
		fmt.Sprintf("%d:%d", listenPort, p.DestinationPort),
	}

	discard := ioutil.Discard
	pf, err := portforward.New(dialer, ports, p.stopChan, readyChan, discard, discard)
	if err != nil {
		return errors.Wrap(err, "Could not port forward into pod")
	}

	go func() {
		errChan <- pf.ForwardPorts()
	}()

	select {
	case err = <-errChan:
		return errors.Wrap(err, "Could not create port forward")
	case <-readyChan:
		return nil
	}

	return nil
}

// Stop a port forward.
func (p *PortForward) Stop() {
	p.stopChan <- struct{}{}
}

func (p *PortForward) getPodName() (string, error) {
	var err error
	if p.ServiceName == "" {
		return "", errors.New("servicename is null")
	}
	svc, err := p.Clientset.CoreV1().Services(p.Namespace).Get(context.TODO(), p.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "get svc err")
	}
	var labels metav1.LabelSelector
	labels.MatchLabels = svc.Labels
	pods, err := p.Clientset.CoreV1().Pods(p.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&labels),
		FieldSelector: fields.OneTermEqualSelector("status.phase", string(v1.PodRunning)).String(),
	})
	if err != nil {
		return "", errors.Wrap(err, "get pod err")
	}
	if len(pods.Items) >= 1 {
		return pods.Items[0].ObjectMeta.Name, nil
	}
	return "", errors.New("not found pod")
}