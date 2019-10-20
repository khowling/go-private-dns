package handler

import (
	"k8s.io/klog/v2"
	"private-dns/endpoint"
	"private-dns/plan"
)

// Handler interface contains the methods that are required
type Handler interface {
	ApplyChanges(changes HashableDNSChanges) error
	ObjectCreated(obj interface{}) HashableDNSChanges 
	ObjectDeleted(obj interface{}) HashableDNSChanges
	ObjectUpdated(objOld, objNew interface{}) HashableDNSChanges
}

// DNSEntry yea
type DNSEntry struct {
	fqdn string
	recordtype string
	ttl int
	ip string
}
// HashableDNSChanges yea
type HashableDNSChanges struct {
	old DNSEntry
	new DNSEntry
}



// HashDNSToPlan Plan is not hashable, so not able to add to workqueue
func HashDNSToPlan(changes HashableDNSChanges) (plan.Changes, bool) {
	apply := plan.Changes{}
	applyIt := false

	if changes.old != (DNSEntry{}) && changes.new != (DNSEntry{}) {
		klog.Info("DNSHandler: ApplyChanges - Update")
		apply = plan.Changes{ 
			UpdateOld: []*endpoint.Endpoint{ endpoint.NewEndpointWithTTL(changes.old.fqdn, changes.old.recordtype , endpoint.TTL(changes.old.ttl), changes.old.ip) },
			UpdateNew: []*endpoint.Endpoint{ endpoint.NewEndpointWithTTL(changes.new.fqdn, changes.new.recordtype , endpoint.TTL(changes.old.ttl), changes.new.ip) },
		}
		applyIt = true
	} else if changes.new != (DNSEntry{}) {
		klog.Info("DNSHandler: ApplyChanges - Add %s  %s", changes.new.fqdn, changes.new.ip)
		apply = plan.Changes{ 
			Create: []*endpoint.Endpoint{ endpoint.NewEndpointWithTTL(changes.new.fqdn, changes.new.recordtype , endpoint.TTL(changes.old.ttl), changes.new.ip) },
		}
		applyIt = true
	} else if changes.old != (DNSEntry{}) {
		klog.Info("DNSHandler: ApplyChanges - Delete")
		apply = plan.Changes{ 
			Delete: []*endpoint.Endpoint{ endpoint.NewEndpointWithTTL(changes.old.fqdn, changes.old.recordtype , endpoint.TTL(changes.old.ttl), changes.old.ip) },
		}
		applyIt = true
	} else {
		klog.Info("DNSHandler: ApplyChanges - Nothing to do")
	}
	return apply, applyIt
}

