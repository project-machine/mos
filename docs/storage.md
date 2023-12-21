# Storage for targes

Following is an example manifest.yaml showing how to specify storage
for targets:

```
storage:
  - label: zot-data
    persistent: true
    nsgroup: "zot"
    size: 30G
  - label: zot-config
    persistent: true
    nsgroup: "zot"
    size: 1G
  - label: zot-tmp
    persistent: false
    nsgroup: "zot"
    size: 1G
  - label: nginx-data
    persistent: true
    nsgroup: "zot"
    size: 1G
targets:
  - service_name: zot
    source: docker://zothub.io/machine/bootkit/demo-zot:0.0.4-squashfs
    version: 1.0.0
    nsgroup: zot
    storage:
      - dest: /zot
        label: zot-data
      - dest: /etc/zot
        label: zot-config
      - dest: /tmp
        label: zot-tmp
  - service_name: nginx
    source: docker://zothub.io/machine/bootkit/demo-nginx:0.0.4-squashfs
    version: 1.0.0
    nsgroup: zot
    storage:
      - dest: /data/zot
        label: zot-data
      - dest: /var/lib/www
        label: nginx-data
```

When a target starts up, its rootfs is an overlay of a writeable tmpfs
over the source OCI image (which itself is an overlay of dmverity-protected
squashfs images).  The writeable overlays are all in a shared partition
mounted at /scratch-writes.  In order to provide persistent storage
across boots, shared storage between containers, or a larger private
ephemeral storage which does not risk filling up /scratch-writes,
extra storage can be requested.

In the above example, four additional storage volumes are requested.  The
30G volume called zot-data will be persistent, so its contents will be
saved across boots. In contrast,  zot-tmp is not persistent, so its contents
will be deleted across reboots.  All four are in the 'nsgroup zot', which
both of the targets, zot and nginx, run in.  The nsgroup is a named
user namespace mapping, so uid 0 will be represented by the same host
uid (for instance 100000) for all.

Note that if nginx were not placed into nsgroup 'zot', it would still
be able to mount zot-data, however all files would appear as
owned by nobody:nogroup, and nginx would get the world access rights.

Each target now has an optional storage section, where it can
specify which volumes it should mount, and where.

On boot, the machine will first create the storage volumes, and uid-shift
them if needed.  If a non-persistent volume already exists, it will be
deleted and recreated.

All storage volumes are created as ext4 filesystems.
