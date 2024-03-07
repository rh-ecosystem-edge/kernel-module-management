#!/bin/bash

if [ -e /var/lib/image_file_day1.tar ]; then
    echo "File /var/lib/image_file_day1.tar found.Nothing to do"
else
    podman pull --authfile /var/lib/kubelet/config.json quay.io/edge-infrastructure/kernel-module-management-worker:latest
    if [ $? -eq 0 ]; then
        echo "Image quay.io/edge-infrastructure/kernel-module-management-worker:latest has been successfully pulled"
    else
        echo "Failed to pull image quay.io/edge-infrastructure/kernel-module-management-worker:latest"
        exit 1
    fi

    echo "File /var/lib/image_file_day1.tar is not on the filesystem, pulling image quay.io/project/repo:some-tag12"
    podman pull --authfile /var/lib/kubelet/config.json quay.io/project/repo:some-tag12
    if [ $? -eq 0 ]; then
        echo "Image quay.io/project/repo:some-tag12 has been successfully pulled"
    else
        echo "Failed to pull image quay.io/project/repo:some-tag12"
        exit 1
    fi
    echo "Saving image quay.io/project/repo:some-tag12 into a file /var/lib/image_file_day1.tar"
    podman save -o /var/lib/image_file_day1.tar quay.io/project/repo:some-tag12
    if [ $? -eq 0 ]; then
        echo "Image quay.io/project/repo:some-tag12 has been successfully save on file /var/lib/image_file_day1.tar, rebooting..."
        reboot
    else
        echo "Failed to save image quay.io/project/repo:some-tag12 to file /var/lib/image_file_day1.tar"
    fi
fi
