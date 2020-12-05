// MIT License
// Copyright (c) 2020 ysicing <i@ysicing.me>

package portforward

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"log"
	"net/http"
	"net/url"
	"os"
)

type PortForward struct {
	Config      *rest.Config
	LocalPort   int
	RemotePort  int
	URL         *url.URL
	Namespace   string
	ServiceName string
	StopCh      chan struct{}
	ReadyCh     chan struct{}
	EnableLog   bool
}

func NewPortForward(config *rest.Config, namespace, servicename string, lport, rport int, enablelog bool) (*PortForward, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	podName, err := getPodName(namespace, servicename, client)
	if err != nil {
		return nil, err
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	return &PortForward{
		Config:     config,
		URL:        req.URL(),
		LocalPort:  lport,
		RemotePort: rport,
		EnableLog:  enablelog,
		StopCh:     make(chan struct{}, 1),
		ReadyCh:    make(chan struct{}),
	}, nil
}

func (p *PortForward) Start() error {
	transport, upgrader, err := spdy.RoundTripperFor(p.Config)
	if err != nil {
		return err
	}

	out := ioutil.Discard
	errOut := ioutil.Discard
	if p.EnableLog {
		out = os.Stdout
		errOut = os.Stderr
	}
	log.Println(p.URL)
	ports := []string{fmt.Sprintf("%d:%d", p.LocalPort, p.RemotePort)}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", p.URL)

	fw, err := portforward.New(dialer, ports, p.StopCh, p.ReadyCh, out, errOut)
	if err != nil {
		return err
	}

	return fw.ForwardPorts()
}

func (p *PortForward) Init() error {
	failure := make(chan error)

	go func() {
		if err := p.Start(); err != nil {
			failure <- err
		}
	}()

	select {
	// if `p.run()` succeeds, block until terminated
	case <-p.ReadyCh:

	// if failure, causing a receive `<-failure` and returns the error
	case err := <-failure:
		return err
	}

	return nil
}

// Stop a port forward.
func (p *PortForward) Stop() {
	close(p.StopCh)
}

func (pf *PortForward) GetStop() <-chan struct{} {
	return pf.StopCh
}

func getPodName(namespace, svcname string, client *kubernetes.Clientset) (string, error) {
	var err error

	svc, err := client.CoreV1().Services(namespace).Get(context.TODO(), svcname, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "get svc err")
	}
	var labels metav1.LabelSelector
	labels.MatchLabels = svc.Labels
	pods, err := client.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&labels),
		FieldSelector: fields.OneTermEqualSelector("status.phase", string(v1.PodRunning)).String(),
	})
	if err != nil {
		return "", errors.Wrap(err, "get pod err")
	}
	if len(pods.Items) >= 1 {
		for _, pod := range pods.Items {
			if isPodReady(&pod) {
				return pod.ObjectMeta.Name, nil
			}
		}
	}
	return "", errors.New("not found pod")
}

func isPodReady(pod *v1.Pod) bool {
	if pod.ObjectMeta.DeletionTimestamp != nil {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
