# Managing Day0/Day1 kmods with KMM

Some kmods might be installed without KMM. In order to enhance KMM's UX we
could, in some cases, help customers to transition the lifecycle management
of they kmods to KMM.

## Definitions

### Day 0

The most basic kmods that are required for a node to become “Ready” in the cluster

**Examples**
* A storage driver that is required in order to mount the rootFS as part of the boot process.
  Vendors will usually work closely with the RHEL team to make those drivers
  in-tree so we won’t worry about them too much here.
* A network driver that is required for the machine to access machine-config-server
  on the bootstrap node to pull the ignition and join the cluster

### Day 1

Kmods that are not required for a node to become “Ready” in the cluster but would
not be able to be unloaded once the node is "Ready".

**Examples**
* An OOT network driver that replaces an outdated in-tree driver to exploit the
  full potential of the NIC while `NetworkManager` depends on it.
  Once the node is "Ready" a customer won't be able to unload the driver because
  of the `NetworkManager` dependency.

### Day2

Kmods that can be dynamically loaded to the kernel or removed from it without
interfering with the cluster infrastructure (such as connectivity).

**Examples**
* GPU operator
* Secondary network adapters
* FPGA

## Layering background

When a day0 kmod was installed in the cluster, it means that “layering” was applied
through MCO and OCP upgrades won’t trigger node upgrades.

Unless a user wants to add new features to its driver, we will never need to
recompile it for them since the node’s OS will remain.

With that being said, MCO has [plans](https://issues.redhat.com/browse/MCO-665)
to rebuild the node images upon a cluster upgrade when Layering is used by MCO.

## Using KMM for managing day0 and day1 kmods

We can leverage KMM to manage the lifecycle of day0/1 kmods without a reboot when the driver allows it.
NOTE: It will not work if the upgrade require a node reboot (when rebuilding initramfs is needed for example)

### 1st option

By treating the kmod as an in-tree driver.

Nothing to do until user wishes to update the kmods.

When the user wishes to upgrade the kmod, they treat it as an in-tree driver
and create a `Module` in the cluster with the `inTreeRemoval` field to unload
the old version of the driver.

**Characteristics**

* Down time - KMM will try to unload and load the kmod on all the selected nodes simultaneously.
* Works in case removing the driver makes the node lose connectivity (because KMM uses a single pod to unload+load the driver)

### 2nd option

By using ordered upgrade.

In this case, user creates a versioned `Module` in the cluster representing the kmods - nothing
will happen since the kmods are already loaded.

When the user wishes to upgrade the kmod, they use the ordered-upgrade feature.

**Characteristics**

* No cluster downtime - the user controls the pace of the upgrade, and how many
  nodes are upgraded at the same time, therefore, an upgrade with no downtime is possible.
* Doesn't work if unloading the driver results in losing connection to the node
  (because KMM will create 2 different worker pods, one for unloading and another for loading which won’t be scheduled)
