#!/bin/bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# GCP startup script that mounts the persistent data disk at
# /opt/yugabyte/data. The data disk is the only state that survives OS
# upgrades; the yba-ctl binary, its config, and the YBA software all live
# on the boot disk and are recreated by the yba_installer provider after
# each VM replacement.
#
# On first boot the disk is formatted as XFS and mounted into a freshly
# created empty /opt/yugabyte/data. On subsequent boots the mountpoint
# guard exits early. fstab persists the mount across reboots.

set -euxo pipefail

# WORKAROUND: YBA's embedded postgres won't start on this AlmaLinux image under
# SELinux enforcing — its data dir lands in unlabeled_t and the daemon is
# denied. Drop SELinux to permissive so the install proceeds. setenforce takes
# effect immediately (no reboot); the config edit persists it across reboots.
# Runs ahead of the mount logic so it still applies on the already-mounted
# early-exit path. Guarded to no-op on images without SELinux (e.g. Ubuntu).
# Remove once the base image ships a working YBA SELinux policy.
if command -v setenforce >/dev/null 2>&1; then
  setenforce 0 || true
fi
if [ -f /etc/selinux/config ]; then
  sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config
fi

# YBA 2025.2.x requires Python 3.10-3.12; AlmaLinux 9 ships 3.9 by default, so
# yba-ctl's python preflight check fails. Install 3.11 from the base repos.
# Guarded so this no-ops on images without dnf (e.g. Ubuntu).
if command -v dnf >/dev/null 2>&1; then
  dnf install -y python3.11 || true
fi

DEVICE="/dev/disk/by-id/google-$(hostname -s)"
MOUNT="/opt/yugabyte/data"

echo "Starting data disk mount: DEVICE=$DEVICE MOUNT=$MOUNT"

if mountpoint -q "$MOUNT"; then
  echo "$MOUNT is already mounted, nothing to do"
  exit 0
fi

if [ ! -b "$DEVICE" ]; then
  echo "ERROR: Cannot find data disk at $DEVICE" >&2
  exit 1
fi

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

if ! grep -q "$DEVICE" /etc/fstab; then
  echo "Adding $DEVICE to /etc/fstab"
  echo "$DEVICE $MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
fi

echo "Data disk mount completed successfully"
