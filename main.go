package main

// IMPORTANT: requires : export GO111MODULE=on
import (
	"context"
	"flag"
	"fmt"
	"os"

	// log system
	"k8s.io/klog/v2"
	"private-dns/endpoint"
	"private-dns/plan"
	"private-dns/provider"

	"os/signal"
	"syscall"

	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
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
	inCluster := flag.Bool("in-cluster", true, "are we running in the cluster?")

	flag.Parse()

	flag.Set("logtostderr", "true")

	if len(*rg) == 0 {
		fmt.Fprintf(os.Stderr, "error: No resource_group\n")
		os.Exit(1)
	}

	p, err := provider.NewAzurePrivateProvider(*inCluster, *rg, *subID)
	if err != nil {
		klog.Fatalf("failed to create NewAzureProvider: %v", err)
	}

	// get the Kubernetes client for connectivity
	client := getKubernetesClient(*inCluster)

	// create the informer so that we can not only list resources
	// but also watch them for all pods in the default namespace
	informer := cache.NewSharedIndexInformer(
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
		0,                 // no resync (period of 0)
		cache.Indexers{},
	)

	// create a new queue so that when the informer gets a resource that is either
	// a result of listing or watching, we can add an idenfitying key to the queue
	// so that it can be handled in the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// add event handlers to handle the three types of events for resources:
	//  - adding new resources
	//  - updating existing resources
	//  - deleting resources
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)

			klog.Infof("Create Service: %s", key)

			//newS := obj.(*api_v1.Service)
			//newIP := ""
			//if len(newS.Status.LoadBalancer.Ingress)>0 { newIP =  newS.Status.LoadBalancer.Ingress[0].IP }

			// This only runs on process Startup, so can confirm here if required?!
			//klog.Printf("got: %v,  %s", newS.Annotations, newIP)

			if err == nil {
				// add the key to the queue for the handler to get
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)

			klog.Infof("Update Service: %s", key)

			oldS := oldObj.(*api_v1.Service)
			newS := newObj.(*api_v1.Service)

			oldFQDN := oldS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
			oldIP := ""
			if len(oldS.Status.LoadBalancer.Ingress) > 0 {
				oldIP = oldS.Status.LoadBalancer.Ingress[0].IP
			}

			oldEntry := oldS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && oldFQDN != "" && oldIP != ""

			newFQDN := newS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
			newIP := ""
			if len(newS.Status.LoadBalancer.Ingress) > 0 {
				newIP = newS.Status.LoadBalancer.Ingress[0].IP
			}

			var CreateNew []*endpoint.Endpoint
			var UpdateOld []*endpoint.Endpoint
			var UpdateNew []*endpoint.Endpoint
			var Delete []*endpoint.Endpoint

			if newS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && newFQDN != "" && newIP != "" {
				if oldEntry {
					if oldFQDN != newFQDN || oldIP != newIP {
						UpdateOld = append(UpdateOld, endpoint.NewEndpointWithTTL(oldFQDN, endpoint.RecordTypeA, endpoint.TTL(3600), oldIP))
						UpdateNew = append(UpdateNew, endpoint.NewEndpointWithTTL(newFQDN, endpoint.RecordTypeA, endpoint.TTL(3600), newIP))
					}
				} else {
					CreateNew = append(CreateNew, endpoint.NewEndpointWithTTL(newFQDN, endpoint.RecordTypeA, endpoint.TTL(3600), newIP))
				}
			} else if oldEntry {
				Delete = append(Delete, endpoint.NewEndpointWithTTL(oldFQDN, endpoint.RecordTypeA, endpoint.TTL(3600), oldIP))
			}

			if len(CreateNew) > 0 || len(UpdateNew) > 0 || len(Delete) > 0 {
				p.ApplyChanges(context.Background(), &plan.Changes{Create: CreateNew, UpdateOld: UpdateOld, UpdateNew: UpdateNew, Delete: Delete})
			}

			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)

			klog.Infof("Delete Service: %s", key)

			oldS := obj.(*api_v1.Service)
			oldFQDN := oldS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
			oldIP := ""
			if len(oldS.Status.LoadBalancer.Ingress) > 0 {
				oldIP = oldS.Status.LoadBalancer.Ingress[0].IP
			}

			if oldS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && oldFQDN != "" && oldIP != "" {
				p.ApplyChanges(context.Background(), &plan.Changes{Delete: []*endpoint.Endpoint{endpoint.NewEndpointWithTTL(oldFQDN, endpoint.RecordTypeA, endpoint.TTL(3600), oldIP)}})
			}

			if err == nil {
				queue.Add(key)
			}
		},
	})

	// construct the Controller object which has all of the necessary components to
	// handle logging, connections, informing (listing and watching), the queue,
	// and the handler
	controller := Controller{
		clientset: client,
		informer:  informer,
		queue:     queue,
		handler:   &TestHandler{},
	}

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	go controller.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}
