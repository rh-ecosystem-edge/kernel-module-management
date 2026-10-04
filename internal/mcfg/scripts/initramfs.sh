#!/bin/bash

set -euo pipefail

stamp_path=/var/opt/kmods/kmm-initramfs.stamp
stage=/etc/kmods/tree
hook_src=/usr/local/bin/kmm-pre-udev.sh
authfile=/var/lib/kubelet/config.json

kernel=$(uname -r)
kernel_tag=$(printf '%s' "$kernel" | tr '+' '-')
dir_name="${DIR_NAME:-/opt}"
image="${CONTAINER_IMAGE}-${kernel_tag}"

stamp_kernel=""
stamp_hash=""
if [ -f "$stamp_path" ]; then
    while IFS= read -r line; do
        case "$line" in
            kernel=*) stamp_kernel=${line#kernel=} ;;
            specHash=*) stamp_hash=${line#specHash=} ;;
        esac
    done < "$stamp_path"
fi

if [ "$stamp_kernel" = "$kernel" ] && [ "$stamp_hash" = "$SPEC_HASH" ]; then
    exit 0
fi

write_stamp() {
    mkdir -p "$(dirname "$stamp_path")"
    printf 'kernel=%s\nspecHash=%s\n' "$kernel" "$SPEC_HASH" > "$stamp_path"
}

if [ "$ROLLBACK" = "true" ]; then
    rpm-ostree initramfs --disable
    write_stamp
    systemctl reboot
    exit 0
fi

if ! podman pull --authfile "$authfile" "$image"; then
    exit 1
fi

cid=$(podman create "$image")
remove_container() {
    podman rm "$cid" >/dev/null 2>&1 || true
}
trap remove_container EXIT

rm -rf "$stage"
mkdir -p "$stage"
podman cp "${cid}:${dir_name}" "${stage}/"

kmod_dir="${stage}${dir_name}/lib/modules/${kernel}"
if [ "$kernel_tag" != "$kernel" ]; then
    tagged_dir="${stage}${dir_name}/lib/modules/${kernel_tag}"
    if [ -d "$tagged_dir" ]; then
        mkdir -p "$(dirname "$kmod_dir")"
        rm -rf "$kmod_dir"
        mv "$tagged_dir" "$kmod_dir"
    fi
fi
mkdir -p "$kmod_dir"
podman cp "${cid}:${MODULES_PATH}/." "${kmod_dir}/"

if [ ! -f "${kmod_dir}/modules.dep" ]; then
    exit 1
fi

mkdir -p "${stage}/usr/lib/dracut/hooks/pre-udev"
cp "$hook_src" "${stage}/usr/lib/dracut/hooks/pre-udev/50-kmm-initramfs.sh"
chmod 755 "${stage}/usr/lib/dracut/hooks/pre-udev/50-kmm-initramfs.sh"

rpm-ostree initramfs --enable --arg=--include --arg="$stage" --arg=/
write_stamp
systemctl reboot
