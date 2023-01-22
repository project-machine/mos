# mos - machine os

mos is the systems management portion of project machine.  See
(this post on OCI based linux)[https://s3hh.wordpress.com/2022/10/27/oci-based-linux/]
for a general overview.  Mos is responsible for

	1. system install
	2. rootfs setup for livecd
	3. secure rootfs setup at boot
	4. service startup
	5. upgrade

Users do not deal with mos directly.  Rather it is called during
initrd and in response to authenticated api calls.  However, to
get more familiar with what mos is doing in the background, you
can look at the tests or try the below experiment using 'machine'
(coming soon).

## General system outline

A mos system must have:

* A configuration directory, usually /config.
* An atomfs store, usually /atomfs-cache.  This contains a (zot)[https://github.com/project-zot/zot] layout of all the container images which will run on the system, including 'hostfs', which will be the root filesystem for the host.
* A 'scratch' directory, usually /scratch-writes.  The atomfs mounts will be set up under this directory, including read-write overlay upperdirs for each.

## /config

The configuration directory contains a directory 'manifest.git'.  The
git directory contains:

* manifest.yaml - this contains an array of SystemTarget.
* for each target in SystemTargets, the content addressed filename of the install manifest which defined it, SHA.yaml.
* for each SHA.yaml,
  * SHA.yaml.signed - signature of SHA.yaml
  * SHA.pem - a certificate verifying the manifest signature

The structures marshalled into manifest.yaml (SystemTargets) and each SHA.yaml
(InstallFile) are defined in pkg/mosconfig/files.go.

Note that each SHA.pem must be signed by a manifest CA cert which
is shipped in the signed initrd.  A properly provisioned host will only
unlock SUDI certificates and LUKS keys to a UKI which is signed by the
right key.  This UKI will include the initrd which contains the manifest
CA, and a mos bringup program which will enforce proper signatures.

## Development

```
go get ./...
make
make test
```

## Layout

pkg/mosconfig contains the code for installing, updating,
and booting a mos system.

cmd/mosctl builds 'mosctl', the frontend binary.

## Test notes

To test the more baroque features, we use an lxc container.  This
must be permitted to mount overlay filesystems.  You can allow this
by adding

```
mount fstype=overlay,
```

to the lxc-container-default-cgns profile.  THe updated /etc/apparmor.d/lxc/lxc-default-cgns
should look like:

```
# Do not load this file.  Rather, load /etc/apparmor.d/lxc-containers, which
# will source all profiles under /etc/apparmor.d/lxc

profile lxc-container-default-cgns flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/lxc/container-base>

  # the container may never be allowed to mount devpts.  If it does, it
  # will remount the host's devpts.  We could allow it to do it with
  # the newinstance option (but, right now, we don't).
  deny mount fstype=devpts,
  mount fstype=cgroup -> /sys/fs/cgroup/**,
  mount fstype=cgroup2 -> /sys/fs/cgroup/**,
  mount fstype=overlay,
}
```
Reload this by calling "sudo systemctl restart apparmor.
