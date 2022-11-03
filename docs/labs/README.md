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

Driver Toolkit image version should correspond to our Openshift server version. Check [DTK documentation](https://github.com/openshift/driver-toolkit#finding-the-driver-toolkit-image-url-in-the-payload) for further information on this.

## Examples

1) [In-cluster build from sources (Multi step)](multistepbuild-kmm.yaml)

   Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel version in the node matches the kernel version set in the literal/regexp field. Once the module is built, copy it in a next step and create the definitive image which will be uploaded to the Internal OpenShift Registry and used as the Module Loader in the selected nodes. This multi step example could be useful if you need the use of extra software which is not present at DTK image but it is in the destination image on which the module files are copied. 
   
2) [In-cluster build from sources (Single step)](singlebuild-kmm.yaml)
 
   Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel version in the node matches the kernel version set in the literal/regexp field. Once the module is built using the same DTK image as a base the definitive image will be uploaded to the Internal OpenShift Registry and used as the Module Loader in the selected nodes.

3) [Load a Pre-built driver image](prebuilt-kmm.yaml)

   Set an existing image which include the prebuilt module files to load if kernel version and selector settings match the KernelMappings regular expression and the selector label/s.

4) [Use a Device Plugin](deviceplugin-kmm.yaml)

   Set an existing image which include software to manage specific hardware in cluster.
   In our example we will be using an image of a device plugin based on [this project](https://github.com/redhat-nfvpe/k8s-dummy-device-plugin) which sets different values on environment variables faking them as "dummy devices".

```console
$ oc exec -it dummy-pod -- printenv | grep DUMMY_DEVICES
DUMMY_DEVICES=dev_3,dev_4
```

## Troubleshooting

A straight way to check if our module is effectively loaded is check within the Module Loader pod:
```console
[enrique@ebelarte crds]$ oc get po
NAME                                READY   STATUS      RESTARTS   AGE
kmm-ci-a-hk472-1-build              0/1     Completed   0          106s
kmm-ci-a-p4qx8-tmpch                1/1     Running     0          24s
[enrique@ebelarte crds]$ oc exec -it kmm-ci-a-p4qx8-tmpch -- lsmod | grep kmm
kmm_ci_a               16384  0
[enrique@ebelarte crds]$ 
```
If you encounter issues when using KMM there are some other places where you can check.

* No container is created after applying my CRD

  - Check if the kernelmapping matches.
  - Check the logs in the KMM controller.

* KMM fails at build

  - Check logs and/or describe build pod.
  - Make sure DTK version matches your Openshift Cluster version.


Alternatively you can use the [must-gather](https://docs.openshift.com/container-platform/4.11/support/gathering-cluster-data.html) tool to collect data about your cluster and more specifically about KMM:
```console
oc adm must-gather --image=quay.io/edge-infrastructure/kernel-module-management-must-gather -- /usr/bin/gather
```
