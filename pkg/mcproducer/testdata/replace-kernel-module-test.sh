#!/bin/bash

echo "before checking image tar file presence"
if [ -e /var/lib/image_file_day1.tar ]; then
    echo "Image file /var/lib/image_file_day1.tar found on the local file system, running kernel-management worker image"
    podman run --user=root --privileged -v /lib/modules:/lib/modules -v /etc/kmm-worker-day1/config.yaml:/etc/kmm-worker/config.yaml -v /var/lib/image_file_day1.tar:/var/lib/image_file_day1.tar quay.io/edge-infrastructure/kernel-module-management-worker:latest kmod load --tarball /etc/kmm-worker/config.yaml
    if [ $? -eq 0 ]; then
        echo "OOT kernel module testKernelModuleName is inserted"
    else
        echo "failed to insert OOT kernel module testKernelModuleName"
    fi
else
    echo "Image file /var/lib/image_file_day1.tar is not present in local registry, will try after reboot"
fi
