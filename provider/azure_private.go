
package provider

// IMPORTANT: requires : export GO111MODULE=on
import (
	"fmt"
	"context"
	"strings"
	// https://godoc.org/github.com/Azure/azure-sdk-for-go/services/privatedns/mgmt/2018-09-01/privatedns
	"github.com/Azure/azure-sdk-for-go/services/privatedns/mgmt/2018-09-01/privatedns"
	
	// using environment-based authentication, call the NewAuthorizerFromEnvironment function to get your authorizer object.
	"github.com/Azure/go-autorest/autorest/azure/auth"

	// Constants for interactions with Azure services (azure.*)
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"

	// log system
	log "github.com/sirupsen/logrus"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"

)

/*
// PrivateZonesClient is an interface of privatedns.PrivateZonesClient that can be stubbed for testing.
type PrivateZonesClient interface {
	ListByResourceGroupComplete(ctx context.Context, resourceGroupName string, top *int32) (result privatedns.PrivateZoneListResultIterator, err error)
}

// RecordType this is
type RecordType string

// PrivateRecordsClient is an interface of dns.RecordClient that can be stubbed for testing.
type PrivateRecordsClient interface {
	ListComplete(ctx context.Context, resourceGroupName string, privateZoneName string, top *int32, recordsetnamesuffix string) (result privatedns.RecordSetListResultIterator, err error)
	Delete(ctx context.Context, resourceGroupName string, privateZoneName string, recordType RecordType, relativeRecordSetName string, ifMatch string) (result autorest.Response, err error)
	CreateOrUpdate(ctx context.Context, resourceGroupName string, privateZoneName string, recordType privatedns.RecordType, relativeRecordSetName string, parameters privatedns.RecordSet, ifMatch string, ifNoneMatch string) (result privatedns.RecordSet, err error)

}
*/

// AzurePrivateProvider implements the DNS provider for Microsoft's Azure cloud platform.
type AzurePrivateProvider struct {
	dryRun        bool
	resourceGroup string
	privateZonesClient  privatedns.PrivateZonesClient
	privateRecordsClient       privatedns.RecordSetsClient
	domainFilter  DomainFilter
	zoneIDFilter DomainFilter
}

// NewAzurePrivateProvider - mimic the NewAzureProvider
func NewAzurePrivateProvider (testDomainFilter string) (*AzurePrivateProvider, error) {
	
	// set environment variable to file location: AZURE_AUTH_LOCATION=./azauth.json
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to read Azure authorizer with error: " + err.Error())
	}

	fs, err := auth.GetSettingsFromFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read Azure authorizer filesettings: " + err.Error())
	}

	privateZonesClient := privatedns.NewPrivateZonesClientWithBaseURI (azure.PublicCloud.ResourceManagerEndpoint, fs.GetSubscriptionID())
	privateZonesClient.Authorizer = authorizer

	privateRecordsClient := privatedns.NewRecordSetsClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, fs.GetSubscriptionID())
	privateRecordsClient.Authorizer = authorizer


	provider := &AzurePrivateProvider{
		resourceGroup:  "kh-aks-arm1",
		dryRun: false,
		privateZonesClient: privateZonesClient,
		privateRecordsClient: privateRecordsClient,
		domainFilter: DomainFilter{[]string{strings.ToLower(strings.TrimSuffix(strings.TrimSpace(testDomainFilter), "."))}, []string{}},
	}

	return provider, nil
}


func (p *AzurePrivateProvider) privateZones() ([]privatedns.PrivateZone, error) {

	var zones []privatedns.PrivateZone

	for list, err := p.privateZonesClient.ListByResourceGroupComplete(context.Background(), p.resourceGroup, nil); list.NotDone(); err = list.Next() {
		if err != nil {
			log.Error(err, "error traverising RG list")
		}

		pzone := list.Value()
		fmt.Printf("Matching %v, with %v,  %T\n", p.domainFilter.filters, *pzone.Name, pzone)


		if pzone.Name == nil {
			continue
		}

		if !p.domainFilter.Match(*pzone.Name) {
			continue
		}

		// if !p.zoneIDFilter.Match(*pzone.ID) {
		// 	continue
		// }

		zones = append(zones, pzone)
	}

	return zones, nil
}


// Records - Get current records inplace
//
// Records the current records or an error if the operation failed.
func (p *AzurePrivateProvider) Records() (endpoints []*endpoint.Endpoint, _ error) {
	zones, err := p.privateZones()
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {

		for list, err := p.privateRecordsClient.ListComplete (context.Background(), p.resourceGroup, *zone.Name, nil, ""); list.NotDone(); err = list.Next() {
			if err != nil {
				log.Error(err, "error traverising RG list")
			}
			
			precord := list.Value()

			if precord.Name == nil || precord.Type == nil {
				log.Error("Skipping invalid record set with nil name or type.")
				continue
			}

			fmt.Printf("Got zone [%v], record type [%v], ttl [%v], name [%v]\n", *zone.Name, *precord.Type, *precord.TTL, *precord.Name)

			recordType := strings.TrimLeft(*precord.Type, "Microsoft.Network/privateDnsZones/")
			if !supportedRecordType(recordType) {
				fmt.Println("dns record type skipping " + recordType)
				continue
			}

			name := formatAzureDNSName(*precord.Name, *zone.Name)
			targets := extractAzureTargets(&precord)
			if len(targets) == 0 {
				log.Errorf("Failed to extract targets for '%s' with type '%s'.", name, recordType)
				continue
			}

			var ttl endpoint.TTL
			if precord.TTL != nil {
				ttl = endpoint.TTL(*precord.TTL)
			}

			ep := endpoint.NewEndpointWithTTL(name, recordType, endpoint.TTL(ttl), targets...)
			log.Debugf(
				"Found %s record for '%s' with target '%s'.",
				ep.RecordType,
				ep.DNSName,
				ep.Targets,
			)
			endpoints = append(endpoints, ep)

		}
	}
	return endpoints, nil
}

// ApplyChanges applies the given changes.
//
// Returns nil if the operation was successful or an error if the operation failed.
func (p *AzurePrivateProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	zones, err := p.privateZones()
	if err != nil {
		return err
	}


	deleted, updated := p.mapChanges(zones, changes)
	p.deleteRecords(deleted)
	p.updateRecords(updated)
	return nil
}

func (p *AzurePrivateProvider) recordSetNameForZone(zone string, endpoint *endpoint.Endpoint) string {
	// Remove the zone from the record set
	name := endpoint.DNSName
	name = name[:len(name)-len(zone)]
	name = strings.TrimSuffix(name, ".")

	// For root, use @
	if name == "" {
		return "@"
	}
	return name
}


func (p *AzurePrivateProvider) deleteRecords(deleted azureChangeMap) {
	// Delete records first
	for zone, endpoints := range deleted {
		for _, endpoint := range endpoints {
			name := p.recordSetNameForZone(zone, endpoint)
			if p.dryRun {
				log.Infof("Would delete %s record named '%s' for Azure DNS zone '%s'.", endpoint.RecordType, name, zone)
			} else {
				log.Infof("Deleting %s record named '%s' for Azure DNS zone '%s'.", endpoint.RecordType, name, zone)
				if _, err := p.privateRecordsClient.Delete(context.Background(), p.resourceGroup, zone, privatedns.RecordType(endpoint.RecordType), name, ""); err != nil {
					log.Errorf(
						"Failed to delete %s record named '%s' for Azure DNS zone '%s': %v",
						endpoint.RecordType,
						name,
						zone,
						err,
					)
				}
			}
		}
	}
}

func (p *AzurePrivateProvider) updateRecords(updated azureChangeMap) {
	for zone, endpoints := range updated {
		for _, endpoint := range endpoints {
			name := p.recordSetNameForZone(zone, endpoint)
			if p.dryRun {
				log.Infof(
					"Would update %s record named '%s' to '%s' for Azure DNS zone '%s'.",
					endpoint.RecordType,
					name,
					endpoint.Targets,
					zone,
				)
				continue
			}

			log.Infof(
				"Updating %s record named '%s' to '%s' for Azure DNS zone '%s'.",
				endpoint.RecordType,
				name,
				endpoint.Targets,
				zone,
			)

			recordSet, err := p.newRecordSet(endpoint)
			if err == nil {
				_, err = p.privateRecordsClient.CreateOrUpdate(
					context.Background(),
					p.resourceGroup,
					zone,
					privatedns.RecordType(endpoint.RecordType),
					name,
					recordSet,
					"",
					"")
			}
			if err != nil {
				log.Errorf(
					"Failed to update %s record named '%s' to '%s' for DNS zone '%s': %v",
					endpoint.RecordType,
					name,
					endpoint.Targets,
					zone,
					err,
				)
			}
		}
	}
}

const (
	azureRecordTTL = 300
)

func (p *AzurePrivateProvider) newRecordSet(endpoint *endpoint.Endpoint) (privatedns.RecordSet, error) {
	var ttl int64 = azureRecordTTL
	if endpoint.RecordTTL.IsConfigured() {
		ttl = int64(endpoint.RecordTTL)
	}
	switch privatedns.RecordType(endpoint.RecordType) {
	case privatedns.A:
		aRecords := make([]privatedns.ARecord, len(endpoint.Targets))
		for i, target := range endpoint.Targets {
			aRecords[i] = privatedns.ARecord{
				Ipv4Address: to.StringPtr(target),
			}
		}
		return privatedns.RecordSet{
			RecordSetProperties: &privatedns.RecordSetProperties{
				TTL:      to.Int64Ptr(ttl),
				ARecords: &aRecords,
			},
		}, nil
	case privatedns.CNAME:
		return privatedns.RecordSet{
			RecordSetProperties: &privatedns.RecordSetProperties{
				TTL: to.Int64Ptr(ttl),
				CnameRecord: &privatedns.CnameRecord{
					Cname: to.StringPtr(endpoint.Targets[0]),
				},
			},
		}, nil
	case privatedns.TXT:
		return privatedns.RecordSet{
			RecordSetProperties: &privatedns.RecordSetProperties{
				TTL: to.Int64Ptr(ttl),
				TxtRecords: &[]privatedns.TxtRecord{
					{
						Value: &[]string{
							endpoint.Targets[0],
						},
					},
				},
			},
		}, nil
	}
	return privatedns.RecordSet{}, fmt.Errorf("unsupported record type '%s'", endpoint.RecordType)
}




type azureChangeMap map[string][]*endpoint.Endpoint


func (p *AzurePrivateProvider) mapChanges(zones []privatedns.PrivateZone, changes *plan.Changes) (azureChangeMap, azureChangeMap) {
	ignored := map[string]bool{}
	deleted := azureChangeMap{}
	updated := azureChangeMap{}
	zoneNameIDMapper := zoneIDName{}
	for _, z := range zones {
		if z.Name != nil {
			zoneNameIDMapper.Add(*z.Name, *z.Name)
		}
	}
	mapChange := func(changeMap azureChangeMap, change *endpoint.Endpoint) {
		zone, _ := zoneNameIDMapper.FindZone(change.DNSName)
		if zone == "" {
			if _, ok := ignored[change.DNSName]; !ok {
				ignored[change.DNSName] = true
				log.Infof("Ignoring changes to '%s' because a suitable Azure DNS zone was not found.", change.DNSName)
			}
			return
		}
		// Ensure the record type is suitable
		changeMap[zone] = append(changeMap[zone], change)
	}

	for _, change := range changes.Delete {
		mapChange(deleted, change)
	}

	for _, change := range changes.UpdateOld {
		mapChange(deleted, change)
	}

	for _, change := range changes.Create {
		mapChange(updated, change)
	}

	for _, change := range changes.UpdateNew {
		mapChange(updated, change)
	}
	return deleted, updated
}






// Helper function (shared with test code)
func formatAzureDNSName(recordName, zoneName string) string {
	if recordName == "@" {
		return zoneName
	}
	return fmt.Sprintf("%s.%s", recordName, zoneName)
}

// Helper function (shared with text code)
func extractAzureTargets(recordSet *privatedns.RecordSet) []string {
	properties := recordSet.RecordSetProperties
	if properties == nil {
		return []string{}
	}

	// Check for A records
	aRecords := properties.ARecords
	if aRecords != nil && len(*aRecords) > 0 && (*aRecords)[0].Ipv4Address != nil {
		targets := make([]string, len(*aRecords))
		for i, aRecord := range *aRecords {
			targets[i] = *aRecord.Ipv4Address
		}
		return targets
	}

	// Check for CNAME records
	cnameRecord := properties.CnameRecord
	if cnameRecord != nil && cnameRecord.Cname != nil {
		return []string{*cnameRecord.Cname}
	}

	// Check for TXT records
	txtRecords := properties.TxtRecords
	if txtRecords != nil && len(*txtRecords) > 0 && (*txtRecords)[0].Value != nil {
		values := (*txtRecords)[0].Value
		if values != nil && len(*values) > 0 {
			return []string{(*values)[0]}
		}
	}
	return []string{}
}
