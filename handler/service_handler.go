package handler

import (
	"k8s.io/klog/v2"
	"private-dns/endpoint"

	"context"

	core_v1 "k8s.io/api/core/v1"
	"private-dns/provider"

)


// DNSHandler is a sample implementation of Handler
type DNSHandler struct{
	Provider provider.Provider
}

// NewDNSHandler returns a Handler.
func NewDNSHandler(inCluster bool, resourceGroup string, subID string) (*DNSHandler, error)  {
	
	klog.Info("NewIngressHandler - Creating Azure Private Provider")

	p, err := provider.NewAzurePrivateProvider(inCluster, resourceGroup, subID)
	if err != nil {
		klog.Fatalf("failed to create NewAzureProvider: %v", err)
		return nil, err
	}

	return &DNSHandler{ Provider: p}, nil
}

// ObjectCreated is called when an object is created
func (t *DNSHandler) ObjectCreated(obj interface{}) HashableDNSChanges {
	klog.Info("DNSHandler.ObjectCreated")
	// assert the type to a Service object to pull out relevant data
	service := obj.(*core_v1.Service)
	klog.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	klog.Infof("    Service Type: %s", service.Spec.Type)
	klog.Infof("    Status: %s", service.Status.LoadBalancer)

	return HashableDNSChanges{}
}

// ObjectDeleted is called when an object is deleted
func (t *DNSHandler) ObjectDeleted(obj interface{}) HashableDNSChanges {
	klog.Infof("DNSHandler.ObjectDeleted: %s", obj)
	oldS := obj.(*core_v1.Service)
	oldFQDN := oldS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
	oldIP :=  ""
	if len(oldS.Status.LoadBalancer.Ingress)>0 { oldIP = oldS.Status.LoadBalancer.Ingress[0].IP }

	if oldS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && oldFQDN != "" && oldIP != "" {
		return HashableDNSChanges{  old: DNSEntry{ fqdn: oldFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: oldIP} } 
	}
	return HashableDNSChanges{}

}

// ObjectUpdated is called when an object is updated
func (t *DNSHandler) ObjectUpdated(objOld, objNew interface{}) HashableDNSChanges {
	klog.Info("DNSHandler.ObjectUpdated")
	oldS := objOld.(*core_v1.Service)
	newS := objNew.(*core_v1.Service)

	oldFQDN := oldS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
	oldIP :=  ""
	if len(oldS.Status.LoadBalancer.Ingress)>0 { oldIP = oldS.Status.LoadBalancer.Ingress[0].IP }

	oldEntry := oldS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && oldFQDN != "" && oldIP != ""

	newFQDN := newS.Annotations["service.beta.kubernetes.io/azure-load-balanver-privatedns-fqdn"]
	newIP := ""
	if len(newS.Status.LoadBalancer.Ingress)>0 { newIP =  newS.Status.LoadBalancer.Ingress[0].IP }

	changes := HashableDNSChanges{}

	klog.Infof("DNSHandler: Got Service, required fqdn=%s", newFQDN)

	if newS.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] == "true" && newFQDN != "" && newIP != "" {
		klog.Info("DNSHandler: Got internal updated Service with fqdn & IP")
		if oldEntry {
			if oldFQDN != newFQDN || oldIP != newIP {
				changes.old = DNSEntry{ fqdn: oldFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: oldIP}
				changes.new = DNSEntry{ fqdn: newFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: newIP}
			}
		} else {
			changes.new = DNSEntry{ fqdn: newFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: newIP}
		}
	} else if (oldEntry) {
		changes.old = DNSEntry{ fqdn: oldFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: oldIP}
	} 

	return changes
}




// ApplyChanges comment
func (t *DNSHandler) ApplyChanges(changes HashableDNSChanges) error {

	apply, applyIt :=HashDNSToPlan (changes)
	
	if applyIt {
		klog.Info("DNSHandler: ApplyChanges")
		return t.Provider.ApplyChanges(context.Background(), &apply)
	}

	klog.Info("DNSHandler: Nothing Applied")
	return nil
}

