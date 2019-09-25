
package main

// IMPORTANT: requires : export GO111MODULE=on
import (
	"os"
	"fmt"
	"context"

	

	// log system
	log "github.com/sirupsen/logrus"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"
	"private-dns/provider"

)

func main() {

	
	if len(os.Args) < 2 {
		log.Fatalf("Please provide your Private DNS Zone domain")
	}
	var testDomain  = os.Args[1]

	p, err := provider.NewAzurePrivateProvider(testDomain)
	if err != nil {
		log.Fatalf("failed to create NewAzureProvider: %v", err)
	}

	endpoints, err := p.Records()

	fmt.Printf ("got existing endpoints %v", endpoints)

	changes := plan.Changes{ Create: []*endpoint.Endpoint{
		endpoint.NewEndpointWithTTL("nginx1." + testDomain, endpoint.RecordTypeA , endpoint.TTL(3600), "192.126.0.19"),
		endpoint.NewEndpointWithTTL("nginx2." + testDomain, endpoint.RecordTypeA , endpoint.TTL(3600), "192.126.0.50"),
	},
							Delete: []*endpoint.Endpoint{
		endpoint.NewEndpointWithTTL("nginx." + testDomain, endpoint.RecordTypeA , endpoint.TTL(3600), "192.126.0.19"),
	} }

	p.ApplyChanges(context.Background(), &changes)
}