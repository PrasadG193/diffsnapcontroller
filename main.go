/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"time"

	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	changeblockservice "example.com/differentialsnapshot/pkg/changedblockservice/changed_block_service"
	"example.com/differentialsnapshot/pkg/controller"
	clientset "example.com/differentialsnapshot/pkg/generated/clientset/versioned"
	informers "example.com/differentialsnapshot/pkg/generated/informers/externalversions"
	"example.com/differentialsnapshot/pkg/signals"
)

var (
	masterURL  string
	kubeconfig string
	csiAddress string
)

const (
	CONTROLLER_NAMESPACE = "testns"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	diffsnapClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building diffsnap clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	diffsnapInformerFactory := informers.NewSharedInformerFactory(diffsnapClient, time.Second*30)

	metricsManager := metrics.NewCSIMetricsManagerForSidecar("cbt-service")

	klog.Infof("csi address: %v", csiAddress)

	//create client
	csiConn, err := connection.Connect(
		csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Errorf("error connecting to CSI driver: %v", err)
		os.Exit(1)
	}

	cbtClient := changeblockservice.NewDifferentialSnapshotClient(csiConn)

	controller := controller.NewController(kubeClient, diffsnapClient,
		diffsnapInformerFactory.Differentialsnapshot().V1alpha1().GetChangedBlockses(),
		cbtClient)

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	diffsnapInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&csiAddress, "csi-address", "/run/csi/socket", "Address of the CSI driver socket.")

}
