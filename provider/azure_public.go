
package provider

// IMPORTANT: requires : export GO111MODULE=on
import (
	"fmt"
	"context"
	"strings"
	// https://godoc.org/github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns
	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	
	"github.com/Azure/go-autorest/autorest"

	// using environment-based authentication, call the NewAuthorizerFromEnvironment function to get your authorizer object.
	"github.com/Azure/go-autorest/autorest/azure/auth"

	// Constants for interactions with Azure services (azure.*)
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"

	// log system
	"k8s.io/klog/v2"

	"private-dns/endpoint"
	"private-dns/plan"
)


// AzureProvider implements the DNS provider for Microsoft's Azure cloud platform.
type AzureProvider struct {
	dryRun        bool
	resourceGroup string
	ZonesClient  dns.ZonesClient
	RecordsClient       dns.RecordSetsClient
}

// NewAzureProvider - mimic the NewAzureProvider
func NewAzureProvider (inCluster bool, resourceGroup string, subId string) (*AzureProvider, error) {
	
	// set environment variable to file location: AZURE_AUTH_LOCATION=./azauth.json
	var authorizer autorest.Authorizer
	var err error
	var subscriptionID string

	if inCluster {
		klog.Info ("Get NewAuthorizerFromEnvironment (from Pod Identity)")
		authorizer, err = auth.NewAuthorizerFromEnvironment()
		if err != nil || authorizer == nil {
			klog.Errorf("failed NewAuthorizerFromEnvironment: %+v", authorizer)
			return nil, fmt.Errorf("failed NewAuthorizerFromEnvironment: %+v", authorizer)
		}
		subscriptionID = subId
	} else {
		// Get Azure auth from azfile.json
		authorizer, err = auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
		if err != nil ||  authorizer == nil {
			return nil, fmt.Errorf("failed to read Azure authorizer with error: " + err.Error())
		}

		fs, err := auth.GetSettingsFromFile()
		if err != nil {
			return nil, fmt.Errorf("failed to read Azure authorizer filesettings: " + err.Error())
		}
		subscriptionID = fs.GetSubscriptionID()
	}

	klog.Infof("Got Subscription %s", subscriptionID)

	ZonesClient := dns.NewZonesClientWithBaseURI (azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	ZonesClient.Authorizer = authorizer

	RecordsClient := dns.NewRecordSetsClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	RecordsClient.Authorizer = authorizer

	provider := &AzureProvider{
		resourceGroup:  resourceGroup,
		dryRun: false,
		ZonesClient: ZonesClient,
		RecordsClient: RecordsClient,
	}

	return provider, nil
}


func (p *AzureProvider) Zones() ([]dns.Zone, error) {

	var zones []dns.Zone

	// The API https://docs.microsoft.com/en-us/rest/api/dns/dns/zones/listbyresourcegroup
	klog.Infof("Call ListByResourceGroupComplete with rg %s", p.resourceGroup)
	for list, err := p.ZonesClient.ListByResourceGroupComplete(context.Background(), p.resourceGroup, nil); list.NotDone(); err = list.Next() {
		if err != nil {
			klog.Error(err, "error traverising RG list")
		}

		pzone := list.Value()
		klog.Infof("Got %v,  %T\n",  *pzone.Name, pzone)

		if pzone.Name == nil {
			continue
		}

		zones = append(zones, pzone)
	}

	return zones, nil
}


// Records - Get current records inplace
//
// Records the current records or an error if the operation failed.
func (p *AzureProvider) Records() (endpoints []*endpoint.Endpoint, _ error) {
	zones, err := p.Zones()
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {

		for list, err := p.RecordsClient.ListAllByDNSZoneComplete (context.Background(), p.resourceGroup, *zone.Name, nil, ""); list.NotDone(); err = list.Next() {
			if err != nil {
				klog.Error(err, "error traverising RG list")
			}
			
			precord := list.Value()

			if precord.Name == nil || precord.Type == nil {
				klog.Error("Skipping invalid record set with nil name or type.")
				continue
			}

			klog.Infof("Got zone [%v], record type [%v], ttl [%v], name [%v]\n", *zone.Name, *precord.Type, *precord.TTL, *precord.Name)

			recordType := strings.TrimLeft(*precord.Type, "Microsoft.Network/DnsZones/")
			if !supportedRecordType(recordType) {
				klog.Infof("dns record type skipping " + recordType)
				continue
			}

			name := formatAzureDNSName(*precord.Name, *zone.Name)
			targets := extractAzureTargets(&precord)
			if len(targets) == 0 {
				klog.Errorf("Failed to extract targets for '%s' with type '%s'.", name, recordType)
				continue
			}

			var ttl endpoint.TTL
			if precord.TTL != nil {
				ttl = endpoint.TTL(*precord.TTL)
			}

			ep := endpoint.NewEndpointWithTTL(name, recordType, endpoint.TTL(ttl), targets...)
			klog.Infof(
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
func (p *AzureProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	zones, err := p.Zones()
	if err != nil {
		return err
	}


	deleted, updated := p.mapChanges(zones, changes)
	p.deleteRecords(deleted)
	p.updateRecords(updated)
	return nil
}

func (p *AzureProvider) recordSetNameForZone(zone string, endpoint *endpoint.Endpoint) string {
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


func (p *AzureProvider) deleteRecords(deleted azureChangeMap) {
	// Delete records first
	for zone, endpoints := range deleted {
		for _, endpoint := range endpoints {
			name := p.recordSetNameForZone(zone, endpoint)
			if p.dryRun {
				klog.Infof("Would delete %s record named '%s' for Azure DNS zone '%s'.", endpoint.RecordType, name, zone)
			} else {
				klog.Infof("Deleting %s record named '%s' for Azure DNS zone '%s'.", endpoint.RecordType, name, zone)
				if _, err := p.RecordsClient.Delete(context.Background(), p.resourceGroup, zone, name, dns.RecordType(endpoint.RecordType), ""); err != nil {
					klog.Errorf(
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

func (p *AzureProvider) updateRecords(updated azureChangeMap) {
	for zone, endpoints := range updated {
		for _, endpoint := range endpoints {
			name := p.recordSetNameForZone(zone, endpoint)
			if p.dryRun {
				klog.Infof(
					"Would update %s record named '%s' to '%s' for Azure DNS zone '%s'.",
					endpoint.RecordType,
					name,
					endpoint.Targets,
					zone,
				)
				continue
			}

			klog.Infof(
				"Updating %s record named '%s' to '%s' for Azure DNS zone '%s'.",
				endpoint.RecordType,
				name,
				endpoint.Targets,
				zone,
			)

			recordSet, err := p.newRecordSet(endpoint)
			if err == nil {
				_, err = p.RecordsClient.CreateOrUpdate(
					context.Background(),
					p.resourceGroup,
					zone,
					name,
					dns.RecordType(endpoint.RecordType),
					recordSet,
					"",
					"")
			}
			if err != nil {
				klog.Errorf(
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

func (p *AzureProvider) newRecordSet(endpoint *endpoint.Endpoint) (dns.RecordSet, error) {
	var ttl int64 = 300
	if endpoint.RecordTTL.IsConfigured() {
		ttl = int64(endpoint.RecordTTL)
	}
	switch dns.RecordType(endpoint.RecordType) {
	case dns.A:
		aRecords := make([]dns.ARecord, len(endpoint.Targets))
		for i, target := range endpoint.Targets {
			aRecords[i] = dns.ARecord{
				Ipv4Address: to.StringPtr(target),
			}
		}
		return dns.RecordSet{
			RecordSetProperties: &dns.RecordSetProperties{
				TTL:      to.Int64Ptr(ttl),
				ARecords: &aRecords,
			},
		}, nil
	case dns.CNAME:
		return dns.RecordSet{
			RecordSetProperties: &dns.RecordSetProperties{
				TTL: to.Int64Ptr(ttl),
				CnameRecord: &dns.CnameRecord{
					Cname: to.StringPtr(endpoint.Targets[0]),
				},
			},
		}, nil
	case dns.TXT:
		return dns.RecordSet{
			RecordSetProperties: &dns.RecordSetProperties{
				TTL: to.Int64Ptr(ttl),
				TxtRecords: &[]dns.TxtRecord{
					{
						Value: &[]string{
							endpoint.Targets[0],
						},
					},
				},
			},
		}, nil
	}
	return dns.RecordSet{}, fmt.Errorf("unsupported record type '%s'", endpoint.RecordType)
}




type azureChangeMap map[string][]*endpoint.Endpoint


func (p *AzureProvider) mapChanges(zones []dns.Zone, changes *plan.Changes) (azureChangeMap, azureChangeMap) {
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
				klog.Infof("Ignoring changes to '%s' because a suitable Azure DNS zone was not found.", change.DNSName)
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
func extractAzureTargets(recordSet *dns.RecordSet) []string {
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
