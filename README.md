### Temporary repo

This repo is work-in-progress, working on adding private-dns support to the exnteral-dns project, but having difficulty getting external-dns to build, so all the code is here until I create a working PR


### to run

Create a Private DNS Zone in a Resource Group https://docs.microsoft.com/en-us/azure/dns/private-dns-overview

NOTE: This code does not need a AKS cluster, It will simulate sync'ing FQDN from a static list.


Create a Service Principle, and a auth file
```
az ad sp create-for-rbac --sdk-auth > azauth.json
```

Now run the program, passing in your Private DNS Zone domain:

```
AZURE_AUTH_LOCATION=./azauth.json go run . "privatedns.private."
```

Now check your Private DNS Service, there should be 2 new A-records


### to build
```
go build
```# go-private-dns
