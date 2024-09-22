#!/bin/bash

kmm_config_file_filepath="$WORKER_CONFIG_FILEPATH"
in_tree_module_to_remove="$IN_TREE_MODULE_TO_REMOVE"
kernel_module="$KERNEL_MODULE"
worker_image="$WORKER_IMAGE"
kernel_module_image="$KERNEL_MODULE_IMAGE"
firmware_files_path="$FIRMWARE_FILES_PATH"
kernel_module_image_tag=$(uname -r)
full_kernel_module_image="$kernel_module_image:$kernel_module_image_tag"
worker_pod_name=kmm-pod
worker_volume_name=kmm-volume

create_kmm_config() {
    # Write YAML content to the file
    cat <<EOF > "$kmm_config_file_filepath"
containerImage: $full_kernel_module_image
inTreeModuleToRemove: $in_tree_module_to_remove
modprobe:
  dirName: /opt
  moduleName: $kernel_module
EOF
    echo "logging contents of the worker config file:"
    cat "$kmm_config_file_filepath"
}

echo "before checking image presence"
if [ -n "$(podman images -q $full_kernel_module_image 2> /dev/null)" ]; then
    echo "Image $full_kernel_module_image found on the local file system, creating kmm config file"
    create_kmm_config
    echo "creating volume"
    podman volume create $worker_volume_name
    podman pod create --name $worker_pod_name
    echo "creating init container"
    copycmd="mkdir -p /tmp/opt/lib/modules && cp -R /opt/lib/modules/* /tmp/opt/lib/modules;"
    if [[ -n "$FIRMWARE_FILES_PATH" ]]; then
      folders=("tmp" "$firmware_files_path");
      path_to_copy_firmware=$(printf '/%s' "${folders[@]%/}")
      copycmd+=" mkdir -p ${path_to_copy_firmware} && \
      cp -R ${firmware_files_path}/* ${path_to_copy_firmware}"
    fi
    podman create \
          --pod $worker_pod_name \
          --init-ctr=always \
          --rm \
          -v $worker_volume_name:/tmp \
          $full_kernel_module_image \
          /bin/sh -c "${copycmd}"
    echo "creating worker container"
    worker_pod_id=$(
    podman create \
      --pod $worker_pod_name\
      --user=root \
      --privileged \
      --rm \
      -v $worker_volume_name:/tmp \
      -v /lib/modules:/lib/modules \
      -v $kmm_config_file_filepath:/etc/kmm-worker/config.yaml \
      $worker_image \
      kmod load /etc/kmm-worker/config.yaml)
    echo "running worker pod"
    podman pod start $worker_pod_name
    if [ $? -eq 0 ]; then
        echo "OOT kernel module $kernel_module is inserted"
    else
        echo "failed to insert OOT kernel module $kernel_module"
    fi
    podman wait $worker_pod_id
    echo "removing kmm-pod"
    podman pod rm $worker_pod_name
    echo "removing volume"
    podman volume rm $worker_volume_name
else
    echo "Image $full_kernel_module_image is not present in local registry, will try after reboot"
fi
