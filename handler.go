package main

import (
	"k8s.io/klog"
	core_v1 "k8s.io/api/core/v1"
)

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(objOld, objNew interface{})
}

// TestHandler is a sample implementation of Handler
type TestHandler struct{}

// Init handles any handler initialization
func (t *TestHandler) Init() error {
	klog.Info("TestHandler.Init")
	return nil
}

// ObjectCreated is called when an object is created
func (t *TestHandler) ObjectCreated(obj interface{}) {
	klog.Info("TestHandler.ObjectCreated")
	// assert the type to a Service object to pull out relevant data
	service := obj.(*core_v1.Service)
	klog.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	klog.Infof("    Service Type: %s", service.Spec.Type)
	klog.Infof("    Status: %s", service.Status.LoadBalancer)
}

// ObjectDeleted is called when an object is deleted
func (t *TestHandler) ObjectDeleted(obj interface{}) {
	klog.Info("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (t *TestHandler) ObjectUpdated(objOld, objNew interface{}) {
	klog.Info("TestHandler.ObjectUpdated")
}