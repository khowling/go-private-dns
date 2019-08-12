module github.com/khowling/private-dns

go 1.12

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.3.0+incompatible

require (
	contrib.go.opencensus.io/exporter/ocagent v0.4.12 // indirect
	github.com/Azure/azure-sdk-for-go v32.3.0+incompatible
	github.com/Azure/go-autorest/autorest v0.8.0
	github.com/Azure/go-autorest/autorest/adal v0.4.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.1.0
	github.com/Azure/go-autorest/autorest/to v0.2.0
	github.com/Azure/go-autorest/autorest/validation v0.1.0 // indirect
	github.com/kubernetes-incubator/external-dns v0.5.15
	github.com/sirupsen/logrus v1.4.2
)
