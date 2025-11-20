#!/bin/bash

kmm_config_file_filepath="$WORKER_CONFIG_FILEPATH"
in_tree_modules_to_remove="$IN_TREE_MODULES_TO_REMOVE"
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
modprobe:
  dirName: /opt
  moduleName: $kernel_module
  firmwarePath: $firmware_files_path
EOF
    
    # Add inTreeModulesToRemove as a proper YAML array if modules are specified
    if [[ -n "$in_tree_modules_to_remove" ]]; then
        echo "inTreeModulesToRemove:" >> "$kmm_config_file_filepath"
        # Convert space-separated list to YAML array format
        for module in $in_tree_modules_to_remove; do
            echo "  - \"$module\"" >> "$kmm_config_file_filepath"
        done
    fi
    
    echo "logging contents of the worker config file:"
    cat "$kmm_config_file_filepath"
}

echo "before checking image presence"
if [ -n "$(podman images -q "$full_kernel_module_image" 2> /dev/null)" ] && \
   [ -n "$(podman images -q "$worker_image" 2> /dev/null)" ]; then
    echo "Images $full_kernel_module_image and $worker_image found on the local file system, creating kmm config file"
    create_kmm_config
    echo "creating volume"
    podman volume create $worker_volume_name
    podman pod create --name $worker_pod_name
    echo "creating init container"
    copycmd="mkdir -p /tmp/opt/lib/modules && cp -R /opt/lib/modules/* /tmp/opt/lib/modules;"
    workerPodArgs="kmod load /etc/kmm-worker/config.yaml"
    mkdir -p /var/lib/firmware
    if [[ -n "$FIRMWARE_FILES_PATH" ]]; then
      folders=("tmp" "$firmware_files_path");
      path_to_copy_firmware=$(printf '/%s' "${folders[@]%/}")
      copycmd+=" mkdir -p ${path_to_copy_firmware} && \
      cp -R ${firmware_files_path}/* ${path_to_copy_firmware}"
      workerPodArgs+=" --firmware-path /var/lib/firmware"
    fi
    podman create \
          --pod $worker_pod_name \
          --init-ctr=always \
          -v $worker_volume_name:/tmp \
          $full_kernel_module_image \
          /bin/sh -c "${copycmd}"
    if [ $? -eq 0 ]; then
        echo "init container for pod ${worker_pod_name} has been created"
    else
        echo "failed to create init container for pod ${worker_pod_name}"
    fi
    worker_pod_id=$(
    podman create \
      --pod $worker_pod_name\
      --user=root \
      --privileged \
      -v $worker_volume_name:/tmp \
      -v /lib/modules:/lib/modules \
      -v $kmm_config_file_filepath:/etc/kmm-worker/config.yaml \
      -v /var/lib/firmware:/var/lib/firmware \
      $worker_image \
      ${workerPodArgs})
    if [ $? -eq 0 ]; then
        echo "worker container for pod ${worker_pod_name} has been created"
    else
        echo "failed to create worker container for pod ${worker_pod_name}"
    fi
    echo "running worker pod"
    podman pod start $worker_pod_name
    if [ $? -eq 0 ]; then
        echo "worker pod has started"
    else
        echo "failed to start worker pod"
    fi
    podman wait $worker_pod_id
    echo "removing kmm-pod"
    podman pod rm $worker_pod_name
    echo "removing volume"
    podman volume rm $worker_volume_name
else
    echo "Image $full_kernel_module_image is not present in local registry, will try after reboot"
fi
