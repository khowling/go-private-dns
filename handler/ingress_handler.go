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
	// assert the type to a Ingress object to pull out relevant data
	//service := obj.(*extensionsv1beta1.Ingress)
	//klog.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	//klog.Infof("    Ingress Rules: %v", service.Spec.Rules[0].Host)
	//klog.Infof("    Status: %s", service.Status.LoadBalancer.Ingress[0].IP)

	/*
	newI := obj.(*extensionsv1beta1.Ingress)

	newFQDN := ""
	if len(newI.Spec.Rules) > 0 { newFQDN = newI.Spec.Rules[0].Host } 

	newIP := ""
	if len(newI.Status.LoadBalancer.Ingress)>0 { newIP =  newI.Status.LoadBalancer.Ingress[0].IP }


	klog.Infof("IngressHandler.ObjectCreated: Got Ingress, required fqdn=%s ip=%s", newFQDN, newIP)
	if newI.Annotations["kubernetes.io/ingress.class"] == "nginx" && newFQDN != "" && newIP != "" {
		klog.Info("IngressHandler: Got new Ingress with fqdn & IP")
		return HashableDNSChanges{new: DNSEntry{ fqdn: newFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: newIP}}
	}
	*/
	return HashableDNSChanges{}
}

// ObjectDeleted is called when an object is deleted
func (t *IngressHandler) ObjectDeleted(obj interface{}) HashableDNSChanges {
	klog.Info("IngressHandler.ObjectDeleted")
	oldI := obj.(*extensionsv1beta1.Ingress)

	oldFQDN := ""
	if len(oldI.Spec.Rules) > 0 { oldFQDN = oldI.Spec.Rules[0].Host } 
	
	oldIP :=  ""
	if len(oldI.Status.LoadBalancer.Ingress)>0 { oldIP = oldI.Status.LoadBalancer.Ingress[0].IP }

	oldEntry := oldI.Annotations["kubernetes.io/ingress.class"] == "nginx" && oldFQDN != "" && oldIP != ""

	if oldEntry {
		return HashableDNSChanges{  old: DNSEntry{ fqdn: oldFQDN, recordtype: endpoint.RecordTypeA , ttl: 3600, ip: oldIP} } 
	}
	return HashableDNSChanges{}

}

// ObjectUpdated is called when an object is updated
func (t *IngressHandler) ObjectUpdated(objOld, objNew interface{}) HashableDNSChanges {
	klog.Info("IngressHandler.ObjectUpdated")
	oldI := objOld.(*extensionsv1beta1.Ingress)
	newI := objNew.(*extensionsv1beta1.Ingress)

	oldFQDN := ""
	if len(oldI.Spec.Rules) > 0 { oldFQDN = oldI.Spec.Rules[0].Host } 

	oldIP :=  ""
	if len(oldI.Status.LoadBalancer.Ingress)>0 { oldIP = oldI.Status.LoadBalancer.Ingress[0].IP }

	oldEntry := oldI.Annotations["kubernetes.io/ingress.class"] == "nginx" && oldFQDN != "" && oldIP != ""

	newFQDN := ""
	if len(newI.Spec.Rules) > 0 { newFQDN = newI.Spec.Rules[0].Host } 

	newIP := ""
	if len(newI.Status.LoadBalancer.Ingress)>0 { newIP =  newI.Status.LoadBalancer.Ingress[0].IP }

	changes := HashableDNSChanges{}

	klog.Infof("IngressHandler.ObjectUpdated: Got Ingress, required fqdn=%s ip=%s", newFQDN, newIP)

	if newI.Annotations["kubernetes.io/ingress.class"] == "nginx" && newFQDN != "" && newIP != "" {
		klog.Info("IngressHandler: Got updated Ingress with fqdn & IP")
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

