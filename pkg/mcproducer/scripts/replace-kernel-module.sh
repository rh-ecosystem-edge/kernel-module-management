#!/bin/bash

kmm_config_file_filepath="$WORKER_CONFIG_FILEPATH"
kernel_module_image_filepath="$KERNEL_MODULE_IMAGE_FILEPATH"
in_tree_module_to_remove="$IN_TREE_MODULE_TO_REMOVE"
kernel_module="$KERNEL_MODULE"
worker_image="$WORKER_IMAGE"

create_kmm_config() {
    # Write YAML content to the file
    cat <<EOF > "$kmm_config_file_filepath"
containerImage: $kernel_module_image_filepath
inTreeModuleToRemove: $in_tree_module_to_remove
modprobe:
  dirName: /opt
  moduleName: $kernel_module
EOF
    echo "logging contents of the worker config file:"
    cat "$kmm_config_file_filepath"
}

echo "before checking image tar file presence"
if [ -e $kernel_module_image_filepath ]; then
    echo "Image file $kernel_module_image_filepath found on the local file system, creating kmm config file"
    create_kmm_config
    echo "running kernel-management worker image"
    podman run --user=root --privileged -v /lib/modules:/lib/modules -v $kmm_config_file_filepath:/etc/kmm-worker/config.yaml -v $kernel_module_image_filepath:$kernel_module_image_filepath $worker_image kmod load --tarball /etc/kmm-worker/config.yaml
    if [ $? -eq 0 ]; then
        echo "OOT kernel module $kernel_module is inserted"
    else
        echo "failed to insert OOT kernel module $kernel_module"
    fi
else
    echo "Image file $kernel_module_image_filepath is not present in local registry, will try after reboot"
fi
