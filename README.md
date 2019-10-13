### Temporary repo

This repo is work-in-progress, working on adding private-dns support to the exnteral-dns project, but having difficulty getting external-dns to build, so all the code is here until I create a working PR

## To build (optional)

To build and run locally

Build the binary
```
$ go build
```

Create a Service Principle (SPN), and a auth file, and grant the SPN the role to list Zones in the resource group, and add DNS records to the zone
```
az ad sp create-for-rbac --sdk-auth > azauth.json
```

Run the program:

```
$ AZURE_AUTH_LOCATION=./azauth.json AZURE_GO_SDK_LOG_LEVEL=DEBUG ./private-dns  -azure-resource-group="kh-aks" -in-cluster=false
```


To build & push a container for deploying into kubenetes, the repo contains a multi stage docker build process 

```
$ docker build .
$ docker login
$ docker tag <IMAGE ID>  <repo>/<project>:<version>
$ docker push <repo>/<project>:<version>
```


## To install into AKS

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
(Following instructions for steps 1-6).  The following assumes your Azure Identity is called `kh-c2-privatedns`.


Create Role Assignment for the role `Private DNS Zone Contributor` on the Private DNS resource for the Managed Identity

```
az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <private-zone-resource-id> --role b12aa53e-6015-4669-85d0-8515ebb3ae7f
```


Create Role Assignment for the role `Reader` on the resource group for the Managed Identity

```
az role assignment create --assignee-principal-type ServicePrincipal --assignee-object-id  <managed-identity-objectID> --scope <resource-group-resource-id> --role acdd72a7-3385-48ef-bd42-f606fba81ae7
```

### Deploy private-dns into your cluster


Update `deploy.yaml` with you build image, resource group and subscription ID

```
kubectl apply -f deploy.yaml
```


### To Test

Deploy a service with the annotations (modify the FQFN annotation to match your DNS Zone)
```
kubectl apply -f example/
```







Now run the program, passing in your Private DNS Zone domain:

```
AZURE_AUTH_LOCATION=./azauth.json go run . "my.akszone.private."
```

Now check your Private DNS Service, create a new Internal LoadBalanver service with the approriate annotations
```
apiVersion: v1
kind: Service
metadata:
  name: internal-app1
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: true
    service.beta.kubernetes.io/azure-private-dns/hostname: "internal-app1.my.akszone.private"
spec:
  type: LoadBalancer
  ports:
  - port: 80
  selector:
    app: internal-app1
```



### to build

This uses the GO Module system.  To recreate the `go.mod` and `go.sum`
* run `go mod init private-dns` to initialise the module files with the module definition of this project 'private-dns'
* run `go get k8s.io/client-go@master` to resolve multiple versions issue

```
go build
```

# Controllers

* Kubernetes `Objects` is a “record of intent” that are persisted to represent the state of your cluster.  They are classified using `kind`
* In addition, every Kubernetes object includes two nested object fields:
    * `spec` describes your desired state for the object & the characteristics that you want the object to have
    * `metadata` is data that helps uniquely identify the object
* PODs have a Owner and can be explisitly set by `OwnerReference` in the spec.   This is the `controller` of the POD.
* A ReplicaSet is one kind of pod controller, and ensures that a specified number of pod replicas are running at any given time
a `Deployment` is a higher-level concept that manages ReplicaSets and provides declarative updates to Pods along with a lot of other useful features. Therefore, we recommend using Deployments instead of directly using ReplicaSets, unless you require custom update orchestration or don’t require updates at all.
* A ReplicaSet identifies new Pods to acquire by using its selector
* A `Controller` is the brains for the kubernetes resources.  There are two types of resources that controllers can “watch, `Core` resources and `custom` resources.You can have a `custom controller` (logic) without a `custom resource` (new datastore kind). Conversely, you can have custom resources without a controller, 
    * the controller “subscribes” to a queue. The controller worker is going to block on a call to get the next item from the queue.  The `informer` is the “link” to the part of Kubernetes that is tasked with handing out these events.  informer responsibility is to register event handlers for the three different types of events: Add, update, and delete

## Custom Controllers
https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#custom-controllers

To work with core resources, when you define your informer you specify a few components
ListWatch — the ListFunc and WatchFunc should be referencing native APIs to list and watch core resources
Controller handlers — the controller should take into account the type of resource that it expects to work with





