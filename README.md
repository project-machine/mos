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

There are some dependencies required for building and running.  The surest
way to get an uptodate list of dependencies is to look at what the github
action workflow (.github/workflows/build.yml) is installing.

```
go get ./...
make
make test
```

## Layout

pkg/mosconfig contains the code for installing, updating, and booting a mos
system.

cmd/mosctl builds 'mosctl', the frontend program used to install and administer
a mos instance.

cmd/mosb builds 'mosb', the program used to build install manifests.

## Using

Most of what mosb and mosctl do is intended to be hidden behind simpler
'machine' commands.  The gist however is as follows:

1. 'mosb manifest publish' will create, sign, and publish an install
   manifest.
2. 'mosctl mount' will mount a remote image which can be used for provisioning
   or installing a host.
3. 'mosctl install' will use the published manifest to install a system (once
   provisioned using 'trust provision').
4. 'mostctl create-boot-fs', during initrd,  will mount an instance of the root
   filesystem on an installed system.
5. 'mosctl update', on an installed and booted system, will update the system
   configuration from a new install manifest.
6. 'mosctl activate', on an installed and booted system, will start or restart
   a service.

A (not yet written) 'mosctl boot' will start all listed services.

A containerized service will be responsible for periodically fetching
(TUF-protected) manifest updates.
