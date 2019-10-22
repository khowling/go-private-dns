package main

// IMPORTANT: requires : export GO111MODULE=on
import (
	"os"
	"fmt"
	"flag"
	

	// log system
	"k8s.io/klog/v2"


	"os/signal"
	"syscall"

	api_v1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"private-dns/handler"
)



// retrieve the Kubernetes cluster client from outside of the cluster
func getKubernetesClient(inCluster bool) kubernetes.Interface {

	var config *rest.Config
	var err error

	if inCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	} else {

		// construct the path to resolve to `~/.kube/config`
		kubeConfigPath := os.Getenv("HOME") + "/.kube/config"
		// create the config from the path
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			klog.Fatalf("getClusterConfig: %v", err)
		}
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("getClusterConfig: %v", err)
	}

	klog.Info("Successfully constructed k8s client")
	return client
}

// main code path
func main() {

	flag.Set("alsologtostderr", "true")

	rg := flag.String("azure-resource-group", "", "Resource Group containing your Azure Private DNS Zone resource")
	subID := flag.String("azure-subscription-id", "", "Subscription Id for in-cluster pod-identity")
	inCluster :=  flag.Bool("in-cluster", true , "are we running in the cluster?")
	publicZone :=  flag.Bool("public-zone", false , "Use a Public DNS Zone")

	flag.Parse()

	flag.Set("logtostderr", "true")

	if len(*rg) == 0 {
		fmt.Fprintf(os.Stderr, "error: No resource_group\n")
        os.Exit(1)
	}

	var dnshandler handler.Handler
	var err error
	if *publicZone {
		dnshandler, err = handler.NewIngressHandler (*inCluster, *rg, *subID)
	} else {
		dnshandler, err = handler.NewDNSHandler(*inCluster, *rg, *subID)
		
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot initialise handler, %v\n", err)
        os.Exit(1)
	}

	// get the Kubernetes client for connectivity
	client := getKubernetesClient(*inCluster)

	// Informer/SharedInformer watches for changes on the current state of Kubernetes objects 
	// and sends events to Workqueue where events are then popped up by worker(s) to process.

	// the SharedInformer helps to create a single shared cache among controllers.  
	// cached resources won't be duplicated and by doing that, the memory overhead of the system is reduced

	// repeatedly retrieving information from the API server can become expensive. Thus, in order to get 
	// and list objects multiple times in code, Kubernetes developers end up using cache which has already 
	// been provided by the client-go library

	// Indexer is a storage interface that lets you list objects using multiple indexing functions


	// The client-go library provides the "Listwatcher" interface that performs an initial list and starts a watch on a particular resource

	// All of these things are consumed in Informer.

	var controller *Controller
	if *publicZone {
		// Public Zone, listen for Ingress
		ingressInformer := cache.NewSharedIndexInformer(
			&cache.ListWatch{
				ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {

					return client.ExtensionsV1beta1().Ingresses("").List(options)
				},
				WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {

					return client.ExtensionsV1beta1().Ingresses("").Watch(options)
				},
			},
			&extensionsv1beta1.Ingress{},
			0,             // no resync (period of 0)
			cache.Indexers{},
		)

		controller = NewController(client, ingressInformer, dnshandler)
	
	} else {
		// Private Zone, listen for Service
		serviceInformer := cache.NewSharedIndexInformer(
			// the ListWatch contains two different functions that our
			// informer requires: ListFunc to take care of listing and watching
			// the resources we want to handle
			&cache.ListWatch{
				ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
					// list all of the pods (core resource) in the deafult namespace
					return client.CoreV1().Services(meta_v1.NamespaceDefault).List(options)
				},
				WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
					// watch all of the pods (core resource) in the default namespace
					return client.CoreV1().Services(meta_v1.NamespaceDefault).Watch(options)
				},
			},
			&api_v1.Service{}, // the target type (Service)
			0,             // no resync (period of 0)
			cache.Indexers{},
		)
		
		controller = NewController(client, serviceInformer, dnshandler)

	}

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	if err := controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}