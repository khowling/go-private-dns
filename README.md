## Purpose

To allow AKS Service/Ingress to automatically register a DNS entry in Azure's managed DNS Zones.

The project can operate in 1 of 2 modes, see the `examples` folder for yaml examples:
 * if `-public-zone=false` (default), this mode will watch for kubenetes `LoadBalancer` type Services with the annotation to use a internal load balancer (private frontend IP) `service.beta.kubernetes.io/azure-load-balancer-internal: "true"` & the annotation with the required DNS FQDN `service.beta.kubernetes.io/azure-dns-zone-fqdn`.  If a matching `Azure Private DNS zone` zone is found in the resource group indicated in the flag `-azure-resource-group`, a DNS record is created in that zone
 * if `-public-zone=true`, this mode will watch for kubenetes nginx Ingress objects with the annotation  `kubernetes.io/ingress.class: "nginx"`.  If that Ingress includes a hosts FQDN, and a matching `Azure DNS zone` top-level zone is found in the resource group indicated in the flag `-azure-resource-group`, a DNS record is created in that zone


## To install into AKS (private zone example)

### Create the DNS Zone

Create a Private DNS Zone in a Resource Group https://docs.microsoft.com/en-us/azure/dns/private-dns-overview
Provide a name (eg 'my.akszone.private')

```
az network private-dns zone create -g <group> -n my.akszone.private
```

NOTE: Note down the resource id
```
"id": "/subscriptions/<sub>/resourceGroups/<group>/providers/Microsoft.Network/privateDnsZones/<zone>",
```

### Create The Identity private-dns will use to update the DNS records in Azure

Install pod-identity into your cluster, details here: https://github.com/Azure/aad-pod-identity#getting-started
(Following instructions for steps 1-6).  

NOTE: The following assumes you've completed steps 1-6 & created an Azure Identity called `kh-c2-privatedns`.


Create `Role Assignment` with the role `Private DNS Zone Contributor` on the Private DNS Zone, required to allow the Managed Identity to add/remove DNS records

```
az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <private-zone-resource-id> --role b12aa53e-6015-4669-85d0-8515ebb3ae7f
```


Create `Role Assignment` with the role `Reader` on the resource group, required to allow the Managed Identity to List all the DNS Zones that can satisfy the required service FQDN

```
az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <resource-group-resource-id> --role acdd72a7-3385-48ef-bd42-f606fba81ae7
```

### Deploy private-dns into your cluster


IMPORTANT: Using the `deploy.yaml` file in the root of this repo, update the file with your `resource group` and `subscription ID`

```
      containers:
      - name: private-dns
        image: khowling/private-dns:0.4
        env:
        - name: AZURE_GO_SDK_LOG_LEVEL
          value: "DEBUG"
        args:
        - --azure-resource-group=<<rg>>
        - --azure-subscription-id=<<subid>>
        
```

Now deploy into your cluster

```
kubectl apply -f deploy.yaml
```


### To Test

Deploy the provided example service with the required annotations 

IMPORTANT: modify the example service in `examples/` with your FQFN annotation to match your DNS Zone
```
metadata:
  name: internal-app1
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
    service.beta.kubernetes.io/azure-dns-zone-fqdn: "internal-app1.my.akszone.private"
```

Now deploy the example service

```
kubectl apply -f examples/internal-lb-with-dns.yaml
```

Within a few minutes, you should see a new DNS record in your Zone


## To build (optional)

### To build and run locally

Build the binary
```
$ go build
```

Create a Service Principle (SPN), and a auth file, and grant the SPN the role to list Zones in the resource group, and add DNS records to the zone
```
az ad sp create-for-rbac --sdk-auth > azauth.json
```

Run the program locally:

```
$ AZURE_AUTH_LOCATION=./azauth.json AZURE_GO_SDK_LOG_LEVEL=DEBUG ./private-dns  -azure-resource-group="kh-aks" -in-cluster=false -public-zone=false
```

### To build a new Image

To build & push a container for deploying into kubenetes, the repo contains a multi stage docker build process 

```
$ docker build .
$ docker login
$ docker tag <IMAGE ID>  <repo>/<project>:<version>
$ docker push <repo>/<project>:<version>
```

To run the image locally

```
docker run --env AZURE_AUTH_LOCATION=./azauth.json khowling/private-dns:0.4  -azure-resource-group="kh-aks" -in-cluster=false -public-zone=false
```
