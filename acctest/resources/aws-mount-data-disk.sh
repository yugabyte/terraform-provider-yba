#!/bin/bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# AWS user-data script that mounts the attached EBS data volume at
# /opt/yugabyte/data. The data volume is the only state that survives OS
# upgrades; the yba-ctl binary, its config, and the YBA software all live on
# the root volume and are recreated by the yba_installer provider after each VM
# replacement.
#
# On first boot the volume is formatted as XFS and mounted into a freshly
# created empty /opt/yugabyte/data. On subsequent boots the mountpoint guard
# exits early. fstab (by-UUID) persists the mount across reboots.
#
# On Nitro instances EBS volumes surface as NVMe devices whose names do not
# match the requested /dev/sdf, so we cannot latch a fixed path: instead we
# discover the lone non-root, unmounted, whole disk. The attachment is a
# separate Terraform resource that can land slightly after first boot, so we
# wait for it to appear.

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

MOUNT="/opt/yugabyte/data"

echo "Starting data disk mount: MOUNT=$MOUNT"

if mountpoint -q "$MOUNT"; then
  echo "$MOUNT is already mounted, nothing to do"
  exit 0
fi

# Identify the root disk so we can exclude it. findmnt gives the root source
# (e.g. /dev/nvme0n1p1); PKNAME walks up to its parent whole disk (nvme0n1).
ROOT_SRC="$(findmnt -no SOURCE / )"
ROOT_DISK="$(lsblk -no PKNAME "$ROOT_SRC" 2>/dev/null || true)"
if [ -z "$ROOT_DISK" ]; then
  # Root is already a whole disk (no partition table).
  ROOT_DISK="$(basename "$ROOT_SRC")"
fi
echo "Root disk: $ROOT_DISK"

# The data volume is attached by a separate Terraform resource that can land
# after first boot. Wait up to ~5min for a non-root, unmounted whole disk to
# appear, re-scanning every iteration (the device may not exist yet).
DEVICE=""
for _ in $(seq 1 60); do
  while read -r name type mountpoint; do
    [ "$type" = "disk" ] || continue
    [ "$name" = "$ROOT_DISK" ] && continue
    [ -n "$mountpoint" ] && continue
    # Skip disks that already have mounted/partitioned children.
    if lsblk -nro MOUNTPOINT "/dev/$name" | grep -q .; then
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

# Persist by-UUID (NVMe device names can reorder across reboots, UUID does not).
# No-op if an entry already exists.
UUID="$(blkid -s UUID -o value "$DEVICE")"
if ! grep -q "$UUID" /etc/fstab; then
  echo "Adding $MOUNT (UUID=$UUID) to /etc/fstab"
  echo "UUID=$UUID $MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
fi

echo "Data disk mount completed successfully"
