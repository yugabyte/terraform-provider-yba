#!/bin/bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# OCI user-data script for the YBA VM: prepares the host for yba-ctl and mounts
# the attached block volume at /opt/yugabyte/data.
#
# cloud-init runs it once, on the VM's first boot. The fstab entry, the SELinux
# config edit and the permanent firewall rules keep the setup across reboots.
#
# The volume attachment can land after first boot, so the script waits for the
# lone non-root, unmounted whole disk instead of a fixed device name.

set -euxo pipefail

# WORKAROUND: YBA's embedded postgres is denied under SELinux enforcing on this
# image (its data dir lands in unlabeled_t). setenforce switches to permissive
# now; the config edit keeps it across reboots. Remove once the image ships a
# working YBA SELinux policy.
if command -v setenforce >/dev/null 2>&1; then
  setenforce 0 || true
fi
if [ -f /etc/selinux/config ]; then
  sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config
fi

# yba-ctl's preflight needs Python 3.10-3.12; Oracle Linux 9 ships 3.9.
if command -v dnf >/dev/null 2>&1; then
  dnf install -y python3.11 || true
  # YBA's provider validation probes NTP servers with nc.
  dnf install -y nmap-ncat || true
fi

# firewalld on these images allows only SSH; open the ports network.tf opens.
if systemctl is-active --quiet firewalld; then
  for port in 443 9000 9090; do
    firewall-cmd --permanent --add-port="$port/tcp"
  done
  firewall-cmd --reload
fi

# Grow the root filesystem to the boot volume size.
if [ -x /usr/libexec/oci-growfs ]; then
  /usr/libexec/oci-growfs -y || true
fi

MOUNT="/opt/yugabyte/data"

echo "Starting data disk mount: MOUNT=$MOUNT"

if mountpoint -q "$MOUNT"; then
  echo "$MOUNT is already mounted, nothing to do"
  exit 0
fi

# findmnt gives the root source (an LVM volume or a partition); lsblk -s walks
# up to the whole disk under it.
ROOT_SRC="$(findmnt -no SOURCE /)"
ROOT_DISK="$(lsblk -snro NAME,TYPE "$ROOT_SRC" | awk '$2 == "disk" { print $1; exit }')"
if [ -z "$ROOT_DISK" ]; then
  ROOT_DISK="$(basename "$ROOT_SRC")"
fi
echo "Root disk: $ROOT_DISK"

# Wait up to ~5min for a non-root, unmounted whole disk to appear.
DEVICE=""
for _ in $(seq 1 60); do
  while read -r name type mountpoint; do
    [ "$type" = "disk" ] || continue
    [ "$name" = "$ROOT_DISK" ] && continue
    [ -n "$mountpoint" ] && continue
    # Skip disks with any child device (partition, LVM), mounted or not.
    if [ "$(lsblk -nro NAME "/dev/$name" | wc -l)" -gt 1 ]; then
      continue
    fi
    DEVICE="/dev/$name"
    break
  done < <(lsblk -dnro NAME,TYPE,MOUNTPOINT)
  if [ -n "$DEVICE" ] && [ -b "$DEVICE" ]; then
    break
  fi
  echo "Waiting for the data volume to attach ..."
  sleep 5
done

if [ -z "$DEVICE" ] || [ ! -b "$DEVICE" ]; then
  echo "ERROR: Cannot find an attached data volume" >&2
  exit 1
fi

echo "Resolved data volume: $DEVICE"

# A replaced VM reattaches the existing, already-formatted volume.
if blkid "$DEVICE"; then
  echo "Disk already formatted: $(blkid "$DEVICE")"
else
  echo "Formatting new disk $DEVICE with xfs"
  mkfs.xfs -L yba-data "$DEVICE"
fi

echo "Mounting $DEVICE to $MOUNT"
mkdir -p /opt/yugabyte
mkdir -p "$MOUNT"
mount -o defaults,nofail "$DEVICE" "$MOUNT"

# By UUID: device names can reorder across reboots.
UUID="$(blkid -s UUID -o value "$DEVICE")"
if ! grep -q "$UUID" /etc/fstab; then
  echo "Adding $MOUNT (UUID=$UUID) to /etc/fstab"
  echo "UUID=$UUID $MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
fi

echo "Data disk mount completed successfully"
