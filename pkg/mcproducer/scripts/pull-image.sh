#!/bin/bash


worker_image="$WORKER_IMAGE"
kernel_module_image="$KERNEL_MODULE_IMAGE"
kernel_module_image_tag=$(uname -r)
full_kernel_module_image="$kernel_module_image:$kernel_module_image_tag"

if [ -n "$(podman images -q $full_kernel_module_image 2> /dev/null)" ]; then
    echo "Image $full_kernel_module_image exist locally. Nothing to do, removing $kmm_config_file_filepath"
    rm -f $kmm_config_file_filepath
else
    podman pull --authfile /var/lib/kubelet/config.json $worker_image
    if [ $? -eq 0 ]; then
        echo "Image $worker_image has been successfully pulled"
    else
        echo "Failed to pull image $worker_image"
        exit 1
    fi

    echo "Pulling image $full_kernel_module_image"
    podman pull --authfile /var/lib/kubelet/config.json $full_kernel_module_image
    if [ $? -eq 0 ]; then
        echo "Image $full_kernel_module_image has been successfully pulled"
    else
        echo "Failed to pull image $full_kernel_module_image"
        exit 1
    fi
    echo "Rebooting..."
    reboot
fi
