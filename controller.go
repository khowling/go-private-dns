package main

import (
	"fmt"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"private-dns/handler"
)

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	clientset kubernetes.Interface

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
	dnshandler   handler.Handler
}


type queueItem struct {
	key interface{}
	changes handler.HashableDNSChanges
}

// NewController returns a new sample controller
func NewController(
	client kubernetes.Interface,
	serviceInformer cache.SharedIndexInformer,
	dnshandler handler.Handler) *Controller {

	// The SharedInformer can't track where each controller is up to (because it's shared), so the controller must provide its own queuing
	// and retrying mechanism (if required). Hence, most Resource Event Handlers simply place items onto a per-consumer workqueue.
	// Workqueue is provided in the client-go library at client-go/util/workqueue.
	// A key uses the format <resource_namespace>/<resource_name>

	controller := &Controller{
		clientset: client,
		informer:  serviceInformer,
		workqueue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		dnshandler:   dnshandler,
	}



	klog.Info("Setting up event handlers")
	// add event handlers to handle the three types of events for resources:
	//  - adding new resources
	//  - updating existing resources
	//  - deleting resources
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {

			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				utilruntime.HandleError(err)
				return
			}
			klog.Infof("Updated Service: %s", key)

			controller.workqueue.Add(queueItem{
				key: key,
				changes: controller.dnshandler.ObjectCreated (obj),
			})
		},
		UpdateFunc: func(old, new interface{}) {

			key, err := cache.MetaNamespaceKeyFunc(new)
			if err != nil {
				utilruntime.HandleError(err)
				return
			}
			klog.Infof("Updated Service: %s", key)

			controller.workqueue.Add(queueItem{
				key: key,
				changes: controller.dnshandler.ObjectUpdated (old, new),
			})
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				utilruntime.HandleError(err)
				return
			}
			klog.Infof("Delete Service: %s", key)

			controller.workqueue.Add(queueItem{
				key: key,
				changes: controller.dnshandler.ObjectDeleted (obj),
			})
		},
	})

	return controller

}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(threadiness int,  stopCh <-chan struct{}) error {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.workqueue.ShutDown()

	klog.Info("Controller.Run: initiating")

	// run the informer to start listing and watching resources
	go c.informer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Controller.Run: Starting workers")

	// run the runWorker method every second with a stop channel
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// HasSynced allows us to satisfy the Controller interface
// by wiring up the informer's HasSynced method to it
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	klog.Info("Controller.runWorker: starting")

	// invoke processNextWorkItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextWorkItem() {
		klog.Info("Controller.runWorker: processing next item")
	}

	klog.Info("Controller.runWorker: completed")
}

// processNextWorkItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextWorkItem() bool {
	klog.Info("Controller.processNextWorkItem: start")

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	event, quit := c.workqueue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.workqueue.Done(event)

	queueItem := event.(queueItem)
	err := c.dnshandler.ApplyChanges(queueItem.changes)

	if err == nil {
		// No error, reset the ratelimit counters
		c.workqueue.Forget(event)
	} else if c.workqueue.NumRequeues(event) < 5 {
		klog.Errorf("Error processing %s (will retry): %v", queueItem.key, err)
		c.workqueue.AddRateLimited(queueItem)
	} else {
		// err != nil and too many retries
		klog.Errorf("Error processing %s (giving up): %v", queueItem.key, err)
		c.workqueue.Forget(queueItem)
		utilruntime.HandleError(err)
	}


	// keep the worker loop running by returning true
	return true
}
