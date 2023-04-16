#!/bin/bash
if podman image exists quay.io/project/repo:some-tag12; then
    echo "Image quay.io/project/repo:some-tag12 found in the local registry.Nothing to do"
else
    echo "Image quay.io/project/repo:some-tag12 not found in the local registry, pulling"
    podman pull --authfile /var/lib/kubelet/config.json quay.io/project/repo:some-tag12
    if [ $? -eq 0 ]; then
        echo "Image quay.io/project/repo:some-tag12 has been successfully pulled, rebooting.."
        reboot
    else
        echo "Failed to pull image quay.io/project/repo:some-tag12"
    fi
fi
