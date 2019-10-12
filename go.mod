module private-dns

go 1.12

require (
	github.com/Azure/azure-sdk-for-go v33.4.0+incompatible
	github.com/Azure/go-autorest v12.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.9.1
	github.com/Azure/go-autorest/autorest/azure/auth v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/kubernetes-incubator/external-dns v0.5.17
	github.com/sirupsen/logrus v1.4.2
	k8s.io/api v0.0.0-20190503184017-f1b257a4ce96
	k8s.io/apimachinery v0.0.0-20190223001710-c182ff3b9841
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/klog v0.0.0-20190306015804-8e90cee79f82
)
