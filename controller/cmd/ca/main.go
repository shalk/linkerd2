package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/linkerd/linkerd2/controller/ca"
	"github.com/linkerd/linkerd2/controller/k8s"
	"github.com/linkerd/linkerd2/pkg/admin"
	"github.com/linkerd/linkerd2/pkg/flags"
	log "github.com/sirupsen/logrus"
)

func main() {
	metricsAddr := flag.String("metrics-addr", ":9997", "address to serve scrapable metrics on")
	controllerNamespace := flag.String("controller-namespace", "linkerd", "namespace in which Linkerd is installed")
	singleNamespace := flag.Bool("single-namespace", false, "only operate in the controller namespace")
	kubeConfigPath := flag.String("kubeconfig", "", "path to kube config")
	proxyAutoInject := flag.Bool("proxy-auto-inject", false, "if true, watch for the add and update events of mutating webhook configurations")
	flags.ConfigureAndParse()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	k8sClient, err := k8s.NewClientSet(*kubeConfigPath)
	if err != nil {
		log.Fatal(err.Error())
	}

	restrictToNamespace := ""
	if *singleNamespace {
		restrictToNamespace = *controllerNamespace
	}

	var k8sAPI *k8s.API
	if *proxyAutoInject {
		k8sAPI = k8s.NewAPI(k8sClient, nil, restrictToNamespace, k8s.Pod, k8s.RS, k8s.MWC)
	} else {
		k8sAPI = k8s.NewAPI(k8sClient, nil, restrictToNamespace, k8s.Pod, k8s.RS)
	}

	controller, err := ca.NewCertificateController(*controllerNamespace, k8sAPI, *proxyAutoInject)
	if err != nil {
		log.Fatalf("Failed to create CertificateController: %v", err)
	}

	stopCh := make(chan struct{})
	ready := make(chan struct{})

	go k8sAPI.Sync(ready)

	go func() {
		log.Info("starting CA")
		controller.Run(ready, stopCh)
	}()

	go admin.StartServer(*metricsAddr, ready)

	<-stop

	log.Info("shutting down")
	close(stopCh)
}
