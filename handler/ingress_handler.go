package handler

import (
	"k8s.io/klog/v2"
	"private-dns/endpoint"


	"context"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"private-dns/provider"

)

// IngressHandler is a sample implementation of Handler
type IngressHandler struct{
	Provider provider.Provider
}

// NewIngressHandler returns a Handler.
func NewIngressHandler(inCluster bool, resourceGroup string, subID string) (*IngressHandler, error) {

	klog.Info("NewIngressHandler - Creating Azure Private Provider")

	p, err := provider.NewAzureProvider(inCluster, resourceGroup, subID)
	if err != nil {
		klog.Fatalf("failed to create NewAzureProvider: %v", err)
		return nil, err
	}

	return &IngressHandler{ Provider: p}, nil
}

// ObjectCreated is called when an object is created
func (t *IngressHandler) ObjectCreated(obj interface{}) HashableDNSChanges {
	klog.Info("IngressHandler.ObjectCreated")
	// assert the type to a Service object to pull out relevant data
	//service := obj.(*extensionsv1beta1.Ingress)
	//klog.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	//klog.Infof("    Ingress Rules: %v", service.Spec.Rules[0].Host)
	//klog.Infof("    Status: %s", service.Status.LoadBalancer.Ingress[0].IP)

	return HashableDNSChanges{}
}

// ObjectDeleted is called when an object is deleted
func (t *IngressHandler) ObjectDeleted(obj interface{}) HashableDNSChanges {
	klog.Infof("IngressHandler.ObjectDeleted: %s", obj)
	oldS := obj.(*extensionsv1beta1.Ingress)

	oldFQDN := ""
	if len(oldS.Spec.Rules) > 0 { oldFQDN = oldS.Spec.Rules[0].Host } 
	
	oldIP :=  ""
	if len(oldS.Status.LoadBalancer.Ingress)>0 { oldIP = oldS.Status.LoadBalancer.Ingress[0].IP }

	oldEntry := oldS.Annotations["kubernetes.io/ingress.class"] == "nginx" && oldFQDN != "" && oldIP != ""

	if oldEntry {
		return HashableDNSChanges{  old: DNSEntry{ fqdn: oldFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: oldIP} } 
	}
	return HashableDNSChanges{}

}

// ObjectUpdated is called when an object is updated
func (t *IngressHandler) ObjectUpdated(objOld, objNew interface{}) HashableDNSChanges {
	klog.Info("IngressHandler.ObjectUpdated")
	oldS := objOld.(*extensionsv1beta1.Ingress)
	newS := objNew.(*extensionsv1beta1.Ingress)

	oldFQDN := ""
	if len(oldS.Spec.Rules) > 0 { oldFQDN = oldS.Spec.Rules[0].Host } 

	oldIP :=  ""
	if len(oldS.Status.LoadBalancer.Ingress)>0 { oldIP = oldS.Status.LoadBalancer.Ingress[0].IP }

	oldEntry := oldS.Annotations["kubernetes.io/ingress.class"] == "nginx" && oldFQDN != "" && oldIP != ""

	newFQDN := ""
	if len(newS.Spec.Rules) > 0 { newFQDN = newS.Spec.Rules[0].Host } 

	newIP := ""
	if len(newS.Status.LoadBalancer.Ingress)>0 { newIP =  newS.Status.LoadBalancer.Ingress[0].IP }

	changes := HashableDNSChanges{}

	klog.Infof("IngressHandler: Got Service, required fqdn=%s", newFQDN)

	if newS.Annotations["kubernetes.io/ingress.class"] == "nginx" && newFQDN != "" && newIP != "" {
		klog.Info("IngressHandler: Got internal updated Service with fqdn & IP")
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
func (t *IngressHandler) ApplyChanges(changes HashableDNSChanges) error {

	apply, applyIt :=HashDNSToPlan (changes)
	
	if applyIt {
		klog.Info("IngressHandler: ApplyChanges")
		return t.Provider.ApplyChanges(context.Background(), &apply)
	}

	klog.Info("IngressHandler: Nothing Applied")
	return nil
}

