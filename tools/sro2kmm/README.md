## Overview

`sro2kmm` is a shell script running an Ansible playbook against an existing OpenShift cluster in order to 
migrate the kernel modules loaded by [Special Resource Operator (SRO)](https://github.com/openshift/special-resource-operator) to the newer [Kernel Module Management operator (KMM)](https://github.com/rh-ecosystem-edge/kernel-module-management).

## Requirements

A running Openshift cluster with:
- Special Resource Operator installed and managing a DaemonSet which loads a kernel module.
- Kernel Module Management operator installed and a Module object running the same kernel module name as in SRO workload.

A computer with:

- Installed [dialog](https://invisible-island.net/dialog/) package.
  Available in most distributions. `dnf install dialog` will work in Red Hat based distributions.
- Openshift Client (`oc`) with access to OpenShift cluster.
- SSH access to OpenShift cluster nodes.
- Kubernetes python packages installed: `python3.8 -m pip install kubernetes --user`
  (Change to your python version accordingly)
- Ansible installed: `dnf install ansible-core` or `dnf install ansible` depending on your workstation OS version.
  **Note** : `ansible-core` does not include kubernetes.core collection as `ansible` does. In case of using
  `ansible-core` as your installation package you should install `community.okd` and `community.general` collections manually to get mandatory `k8s_auth` and `json_query` modules:
```console
ansible-galaxy collection install community.okd community.general
```

Further info at [Ansible Documentation](https://docs.ansible.com/ansible/latest/installation_guide/index.html).

## Warnings

- This script reboots the specified nodes at `inventory_hosts` file so even if it does a sequentally reboot you are responsible for checking in advance that no production service could be interrupted by the action of the script.
- If you set the option to delete the `SpecialResource` created by SRO, notice that all imagestreams created beforehand by SRO will be deleted once the script ends so you should have a different source for the images you want to use with KMM.

## Configuration

The only configuration you should really change is the auth file `tools/sro2kmm/vars/ocp.yaml`
which includes administrator credentials for your cluster and the `inventory_hosts` file which sets the nodes on which SRO2KMM will run.

## Usage

Run `./sro2kmm [sr_name] --menu` script to begin the migration.

There are three possible arguments to be used by the script. First is the name of the `SpecialResource` that you intend to migrate, which is a mandatory argument.
Second argument is related to the settings applied to the kubernetes [k8s_drain](https://docs.ansible.com/ansible/latest/collections/kubernetes/core/k8s_drain_module.html#parameters) module, specifically the delete options used by it.
Optional delete settings can be set by providing the `--menu` or `m` arguments to the script, which will show a checklist for the user to choose:

- `delete_emptydir_data`: continue even if there are pods using emptyDir (local data that will be deleted when the node
 is drained).
- `disable_eviction`: forces drain to use delete rather than evict.
- `force`: continue even if there are pods not managed by a ReplicationController, Job, or DaemonSet.
- `ignore_daemonsets`: ignore DaemonSet-managed pods.

Third argument to `sro2kmm` is the verbose flag to the ansible playbook.
`-v` or `--verbose` will output full ansible verbose.

DaemonSets owned by the specified SpecialResource will be shown in the main selection menu. 
Also a next menu will be shown so you can select containers on which migration patch should
be applied.
Dialog will additionally ask if SpecialResource must be deleted automatically after
migration process.
After user's choice, the playbook will be run using the file `inventory_hosts`.

`cluster_inventory.sh` is provided to create a `workers` inventory group where the roles will
 be run in order to manage multiple nodes which could be difficult to handle manually.
`cluster_inventory.sh` can be customized to fit your needs and host group names but
you will need to remove the comment to invocate it at `sro2kmm` main script as it is commented
by default.

A directory called `data` will be created by the `sro2kmm` script to save and prepare the 
necessary files for your custom configuration.

## Workflow

Running main playbook triggers the following process:

### From machine running sro2kmm 

- Login to OCP cluster
- Dump a description of existing SRO DaemonSet to a local file `sro_ds_backup.yaml`
- Patch SRO DaemonSet setting **UpdateStrategy** to **OnDelete** and switching the `modprobe` command for a 
`sleep infinity` command.
- Install needed python modules (pyyaml, kubernetes)
- Login to OCP cluster to get an API key for next tasks
- Cordon node to not allow new workloads
- Drain node to move existing workloads to other nodes
- Reboot node
- Patch SRO DaemonSet a second time to remove the PreStop hook `modprobe -r`
- Delete old SRO managed pods
- Uncordon node

[TODO]:
- Numeric dialog menu for timeout delete settings.
- Checks for old SRO pods correctly removed.
