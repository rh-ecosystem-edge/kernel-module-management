#!/bin/bash


kernel_module_image_filepath="$KERNEL_MODULE_IMAGE_FILEPATH"
worker_image="$WORKER_IMAGE"
kernel_module_image="$KERNEL_MODULE_IMAGE"

if [ -e $kernel_module_image_filepath ]; then
    echo "File $kernel_module_image_filepath found.Nothing to do, the file was handled, removing it"
    rm -f $kernel_module_image_filepath
else
    podman pull --authfile /var/lib/kubelet/config.json $worker_image
    if [ $? -eq 0 ]; then
        echo "Image $worker_image has been successfully pulled"
    else
        echo "Failed to pull image $worker_image"
        exit 1
    fi

    echo "File $kernel_module_image_filepath is not on the filesystem, pulling image "
    podman pull --authfile /var/lib/kubelet/config.json $kernel_module_image
    if [ $? -eq 0 ]; then
        echo "Image $kernel_module_image has been successfully pulled"
    else
        echo "Failed to pull image $kernel_module_image"
        exit 1
    fi
    echo "Saving image $kernel_module_image into a file $kernel_module_image_filepath"
    podman save -o $kernel_module_image_filepath $kernel_module_image
    if [ $? -eq 0 ]; then
        echo "Image $kernel_module_image has been successfully save on file $kernel_module_image_filepath, rebooting..."
        reboot
    else
        echo "Failed to save image $kernel_module_image to file $kernel_module_image_filepath"
    fi
fi
