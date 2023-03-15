#!/bin/bash
echo "before checking podman images"
if podman images list | grep quay.io/project/repo | grep -q some-tag12; then
    echo "Image quay.io/project/repo:some-tag12 found in the local registry, removing in-tree kernel module"
    podman run --privileged --entrypoint modprobe quay.io/project/repo:some-tag12 -rd /opt testKernelModuleName
    if [ $? -eq 0 ]; then
            echo "Succesffully removed the in-tree kernel module testKernelModuleName"
    else
            echo "failed to remove in-tree kernel module testKernelModuleName"
    fi
    echo "Running container image to insert the oot kernel module testKernelModuleName"
    podman run --privileged --entrypoint modprobe quay.io/project/repo:some-tag12 -d /opt testKernelModuleName
    if [ $? -eq 0 ]; then
            echo "OOT kernel module testKernelModuleName is inserted"
    else
            echo "failed to insert OOT kernel module testKernelModuleName"
    fi
else
   echo "Image quay.io/project/repo:some-tag12 is not present in local registry, will try after reboot"
fi
