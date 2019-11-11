## Purpose

A Kubernetes `Custom Controller` to watch for Services or Ingress objects and automatically synchronize DNS records in Azure's managed public or private DNS Zones.


The Controller can operate either operate in `private` or `public` mode: 

### Private


If `-public-zone=false` (default), the controller will watch for kubenetes `LoadBalancer` type `Services` that provisions an internal load balancer that uses a private frontend IP. The following service annotations will be required:
  * `service.beta.kubernetes.io/azure-load-balancer-internal: "true"` 
  * `service.beta.kubernetes.io/azure-dns-zone-fqdn: "<the required service fqdn>`

If an appropriate `Azure Private DNS zone` is found to host the fqdn, a DNS record will be synchronized in that zone.  NOTE:  the DNS Zone's resource group must be provided in the flag `-azure-resource-group`

### Public

If `-public-zone=true`, the controller will watch for kubernetes `Ingress` objects that include a ingres.class annotation & a public IP address.  For example:
  * `kubernetes.io/ingress.class: nginx` 
  * `kubernetes.io/ingress.class: azure/application-gateway` 

IMPORTANT: Ensure you are using a ingress controller that publishes the Public IP address back onto the Ingress object, for example, with `nginx` use the paramter `controller.publishService.enabled`, and for Azure application gateway, the 1.0.0 or greater GA controller version. 

If an appropriate `Azure DNS zone` is found to host the fqdn, a DNS record will be synchronized in that zone.  NOTE:  the DNS Zone's resource group must be provided in the flag `-azure-resource-group`

### Notes

Examples of Service and Ingress annotations can be found in the `examples` folder.

This standalown project has been created as I've been having difficulties updating the `external-dns` project with Private Zone support due to build issues.  The goal will be to merge these features into external-dns when possible.


## To install into AKS (Private Zone example)


### Create the DNS Zone

1. Create a Private DNS Zone in a Resource Group https://docs.microsoft.com/en-us/azure/dns/private-dns-overview
Provide a name (eg 'my.akszone.private')

    ```
    az network private-dns zone create -g <group> -n my.akszone.private
    ```

    NOTE: Note down the resource id
    ```
    "id": "/subscriptions/<sub>/resourceGroups/<group>/providers/Microsoft.Network/privateDnsZones/<zone>",
    ```

### Install the Zone controller into your cluster (using Helm - recommended)

2. Install pod-identity into your cluster & create a managed identity that the controller will use to update your DNS Zone.  Follow ONLY Steps `1. Create the Deployment` & `2. Create an Azure Identity`  from here: https://github.com/Azure/aad-pod-identity#getting-started.


3. Assign the required RBAC to the managed identity

    Create `Role Assignment` with the role `Private DNS Zone Contributor` on the Private DNS Zone, required to allow the Managed Identity to add/remove DNS records

    ```
    az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <private-zone-resource-id> --role b12aa53e-6015-4669-85d0-8515ebb3ae7f
    ```


    Create `Role Assignment` with the role `Reader` on the resource group, required to allow the Managed Identity to List all the DNS Zones that can satisfy the required service FQDN

    ```
    az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <resource-group-resource-id> --role acdd72a7-3385-48ef-bd42-f606fba81ae7
    ```

3. Now, install the Zone Controller helm chart

  ```
  helm install  https://github.com/khowling/go-private-dns/blob/master/helm/azure-dns-controller-0.1.0.tgz?raw=true \
          --name template-dns-controller \
          --set controllerConfig.resourceGroup=<resource group name where your DNS Zone is located> \
          --set controllerConfig.subscriptionId=<subscriptionId> \
          --set managedIdentity.identityClientId=<the 'clientId' of the managed identity> \
          --set managedIdentity.identityResourceId=<the 'id' of the managed identity>
  ```




### Deploy manually (not recommended)

1. Fully install pod-identity, follow steps 1-6

2. Create the role assignements (as above)

3. Using the `deploy.yaml` file in the root of this repo, change the `public-zone` to `true`, and update the file with your `resource group` and `subscription ID`

```
      containers:
      - name: private-dns
        image: khowling/private-dns:0.5
        env:
        - name: AZURE_GO_SDK_LOG_LEVEL
          value: "DEBUG"
        args:
        - --azure-resource-group=<<rg>>
        - --azure-subscription-id=<<subid>>
        - --public-zone=false
        
```

Now deploy into your cluster

```
kubectl apply -f deploy.yaml
```


### To Test

Deploy the provided example service with the required annotations 

Modify the example service in `examples/` with your FQFN annotation to match your DNS Zone
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
docker run --env AZURE_AUTH_LOCATION=./azauth.json khowling/private-dns:0.5  -azure-resource-group="kh-aks" -in-cluster=false -public-zone=false
```
