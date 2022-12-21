# mos - machine os

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

