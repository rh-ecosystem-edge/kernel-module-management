# Using DTK as a base image for driver-containers in-cluster builds

# Background
OCP deploys an imagestream for the [driver-toolkit](https://github.com/openshift/driver-toolkit) in the cluster. This imagestream contains 2 types of tags:
* A tag representing the RHCOS version which that driver-toolkit image was build for
* The `latest` tag which point to the driver-toolkit image that was build for the latest RHCOS version present in the cluster.

Here is how the imagestream looks like after a cluster upgrade:
```
apiVersion: image.openshift.io/v1
kind: ImageStream
…
spec:
…
  tags:
  - annotations: null
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:84edddb4322cfe50d8545c41014a8f5595a4260a20d9a269e144909c5949582b
    generation: 2
    importPolicy:
      scheduled: true
    name: 410.84.202207140725-0
    referencePolicy:
      type: Source
  - annotations: null
    from:
      kind: DockerImage
      name: registry.ci.openshift.org/ocp/4.11-2022-07-27-013908@sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
    generation: 4
    importPolicy:
      scheduled: true
    name: 411.86.202207260413-0
    referencePolicy:
      type: Source
  - annotations: null
    from:
      kind: DockerImage
      name: registry.ci.openshift.org/ocp/4.11-2022-07-27-013908@sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
    generation: 4
    importPolicy:
      scheduled: true
    name: latest
    referencePolicy:
      type: Source
status:
  dockerImageRepository: image-registry.openshift-image-registry.svc:5000/openshift/driver-toolkit
  tags:
  - items:
    - created: "2022-07-27T02:20:47Z"
      dockerImageReference: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:84edddb4322cfe50d8545c41014a8f5595a4260a20d9a269e144909c5949582b
      generation: 2
      image: sha256:84edddb4322cfe50d8545c41014a8f5595a4260a20d9a269e144909c5949582b
    tag: 410.84.202207140725-0
  - items:
    - created: "2022-07-27T02:55:02Z"
      dockerImageReference: registry.ci.openshift.org/ocp/4.11-2022-07-27-013908@sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
      generation: 4
      image: sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
    tag: 411.86.202207260413-0
  - items:
    - created: "2022-07-27T02:55:02Z"
      dockerImageReference: registry.ci.openshift.org/ocp/4.11-2022-07-27-013908@sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
      generation: 4
      image: sha256:484c9fe9900020e9b8f5601cbe86d054fe47a6f16e8d9dc6ecbfd58bffd1ad4b
    - created: "2022-07-27T02:20:47Z"
      dockerImageReference: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:84edddb4322cfe50d8545c41014a8f5595a4260a20d9a269e144909c5949582b
      generation: 2
      image: sha256:84edddb4322cfe50d8545c41014a8f5595a4260a20d9a269e144909c5949582b
    tag: latest
```

We can see that:
* We have a different tag for each RHCOS version that the cluster has known.
* The ‘latest’ tag point to the most recent RHCOS tag in the imagestream
* The `status` field of the imagestream keeps an array of images for each tag with the full digest history used for that tag
    * The latest tag was first pointing to `410.84.202207140725-0` tag and then move to point to `411.86.202207260413-0` tag.


## Node reconciler flow
Create and update an in-memory mapping `kernelVersion` → `rhcosVersion` (latest `rhcosVersion` with that kernel).
* The `kernelVersion` can be found on the node object at `node.status.nodeInfo.kernelVersion`
* The `rhcosVersion` can be found on the node object at `node.status.nodeInfo.osImage`

## Module reconciler flow

When a build pod is created, or when a cluster upgrade kicks, KMMO will get the list of kernels running in the cluster from the node labels.
For each kernel in the cluster, if a build is required, get the matching `rhcosVersion` from the `kernelVersion` → `rhcosVersion` mapping.
KMMO will use that RHCOS version as the imagestream tag in order to build/rebuild the driver container.

## Dockerfile examples
If the user want us to pull the correct DTK image on builds/cluster-upgrade, he will need to put a placeholder in the Dockerfile for KMMO to substitute. In case this placeholder isn't present in the Dockerfile, KMMO will pull the base image specified by the user.

#### Dockerfile which let KMMO find the correct DTK image.
```
ARG DTK-AUTO
FROM ${DTK-AUTO}
RUN gcc ...
...
```

#### Dockerfile with 0-magic.
```
FROM user.registry.com/org/user-base-image:tag
RUN gcc ...
...
```

## Disconnected clusters
This approach reduce the complexity of the flow in disconnected clusters as we don’t need to pull the driver-toolkit image manually to get the kernel RPMs installed in it. Only the buildconfig will pull the DTK image, therefore, it will be redirected if the user has set an `ImageSourceContentPolicy` in the cluster but we will still need to access the user's CA certificates in the cluster.

Putting aside DTK's consumption itself, we also need to read some `ImageContextSourcePolicy` data in order to check if the driver container already exist prior to building it in-cluster.

## Pre-built driver containers

This case is simpler because unlike KMMO, which is surprised by a cluster upgrade when he sees a node with a new kernel label, a user upgrading his cluster should know to what OCP version he is upgrading to, therefore, it can get the DTK image by running

`oc adm release info quay.io/openshift-release-dev/ocp-release:<cluster-version>-x86_64 --image-for=driver-toolkit` and getting the relevant driver-toolkit image for is pre-built driver-container.

## Future possible usage
KMMO can watch the image stream and act upon image changes. For example KMMO can rebuild a driver-container with a newer version (for the same z-stream) available in the imagestream.

## Benefits of that approach
Using the imagestream which is created and managed for us in the cluster.

It doesn't require any changes to the API.

It is not requiring any additional controllers but is just extending the node-controller and the module-controller slightly.

It is self contained in a way that the node-controller doesn't need to access any other resource than the node and the module-controller doesn't need to access any other resource than the module.
