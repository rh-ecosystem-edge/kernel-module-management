# KMM Labs on OpenShift 

KMM Labs is a series of examples and use cases of Kernel Module Management Operator running on an Openshift cluster.

Kernel Module Management Operator a.k.a KMM allows to run DriverContainers on Openshift clusters and optionally build 
in-cluster out-of-tree drivers from sources or even load DevicePlugins needed to manage special hardware on the cluster.

Once KMM is installed a new ReplicaSet will be deployed and a new pod running in the `kmm-operator-system` namespace will include the KMM controller.

Also a new CRD called Module will be available in our cluster API which
will be defined and created by the user. This definition will include:

* The name of the module object

* The name of the kernel module to load in node

* The mappings for different kernel versions
   
* Repository auth settings

* The Dockerfile steps to be followed in case of in-cluster build

* The selector 


In the case of in-cluster builds we leverage the existence of the [Driver Toolkit](https://github.com/openshift/driver-toolkit) which is a container 
image that includes all the necessary packages and tools to build a kernel module within the OpenShift cluster.

## Examples

1) In-cluster build from sources (Multi step)

   Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel version in the node matches the kernel version set in the literal/regexp field. Once the module is built, copy it in a next step and create the definitive image which will be uploaded to the Internal OpenShift Registry and used as the Module Loader in the selected nodes.
   
2) In-cluster build from sources (Single step)
 
   Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel version in the node matches the kernel version set in the literal/regexp field. Once the module is built using the same DTK image as a base the definitive image will be uploaded to the Internal OpenShift Registry and used as the Module Loader in the selected nodes.

3) Load a Pre-built driver image

   Set an existing image for modules to load when matching kernel version and selector settings.

4) Use a Device Plugin

   Set an existing image which include software to manage specific hardware in cluster.

## Troubleshooting

* No container is created after applying my CRD
  Check if the kernelmapping matches.
  Check the logs in the KMM controller.


