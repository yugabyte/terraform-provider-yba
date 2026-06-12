#!/bin/bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Azure custom-data script that mounts the attached managed data disk at
# /opt/yugabyte/data. The data disk is the only state that survives OS
# upgrades; the yba-ctl binary, its config, and the YBA software all live on
# the OS disk and are recreated by the yba_installer provider after each VM
# replacement.
#
# On first boot the disk is formatted as XFS and mounted into a freshly created
# empty /opt/yugabyte/data. On subsequent boots the mountpoint guard exits
# early. fstab (by-UUID) persists the mount across reboots.
#
# The data disk is attached at LUN 0, so it is resolved via the stable
# /dev/disk/azure/scsi1/lun0 symlink the Azure VM agent (waagent) creates.

set -euxo pipefail

# WORKAROUND: YBA's embedded postgres won't start on this AlmaLinux image under
# SELinux enforcing — its data dir lands in unlabeled_t and the daemon is
# denied. Drop SELinux to permissive so the install proceeds. setenforce takes
# effect immediately (no reboot); the config edit persists it across reboots.
# Runs ahead of the mount logic so it still applies on the already-mounted
# early-exit path. Guarded to no-op on images without SELinux.
# Remove once the base image ships a working YBA SELinux policy.
if command -v setenforce >/dev/null 2>&1; then
  setenforce 0 || true
fi
if [ -f /etc/selinux/config ]; then
  sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config
fi

# YBA 2025.2.x requires Python 3.10-3.12; AlmaLinux 9 ships 3.9 by default, so
# yba-ctl's python preflight check fails. Install 3.11 from the base repos.
# Guarded so this no-ops on images without dnf.
if command -v dnf >/dev/null 2>&1; then
  dnf install -y python3.11 || true
fi

LUN_LINK="/dev/disk/azure/scsi1/lun0"
MOUNT="/opt/yugabyte/data"

echo "Starting data disk mount: LUN_LINK=$LUN_LINK MOUNT=$MOUNT"

if mountpoint -q "$MOUNT"; then
  echo "$MOUNT is already mounted, nothing to do"
  exit 0
fi

# The data disk is attached at LUN 0 by a separate Terraform resource
# (azurerm_virtual_machine_data_disk_attachment), which can land slightly after
# this VM's first boot — so the waagent lun0 symlink may not exist yet when
# cloud-init runs. Re-resolve it on every iteration (resolving once up front
# latches an empty path and loops forever) and wait up to ~5min for the disk to
# appear and the symlink to point at a real block device.
DEVICE=""
for _ in $(seq 1 60); do
  DEVICE="$(readlink -f "$LUN_LINK" 2>/dev/null || true)"
  if [ -n "$DEVICE" ] && [ -b "$DEVICE" ]; then
    break
  fi
  echo "Waiting for data disk at $LUN_LINK ..."
  sleep 5
done

if [ -z "$DEVICE" ] || [ ! -b "$DEVICE" ]; then
  echo "ERROR: Cannot find data disk at $LUN_LINK" >&2
  exit 1
fi

echo "Resolved data disk: $LUN_LINK -> $DEVICE"

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

# Persist by-UUID (LUN device paths are stable, but UUID survives controller
# reordering). No-op if an entry already exists.
UUID="$(blkid -s UUID -o value "$DEVICE")"
if ! grep -q "$UUID" /etc/fstab; then
  echo "Adding $MOUNT (UUID=$UUID) to /etc/fstab"
  echo "UUID=$UUID $MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
fi

echo "Data disk mount completed successfully"
