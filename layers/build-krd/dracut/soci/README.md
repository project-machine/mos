# soci root dracut module.
This dracut module allows using an soci layer as the 'root' filesystem.
It depends on `mosctl soci mount` to do most of the heavy lifting.

It supports the following argument format for 'root='.  Order of the
comma (,) delimited key/value pairs is not important.

    root=soci:name=<layer-name>,dev=<device>[,path=path][,mdpath=<path>][,repo=<url>|local]

 * 'name' is required, and is the layer name of the soci layer.  The name
    is the value of the 'org.opencontainers.image.ref.name' attribute.
    In some cases that may be of the form 'name:tag'.  So if you use
    a name like 'rootfs:latest', you will have to specify 'rootfs:latest' here.

 * 'dev' refers to the device to mount in order to access the soci repository.
   It supports the following mechanisms for identifying the device:

     * ID=name - entry in /dev/disk/by-id/
     * UUID=name - filesystem UUID (/dev/disk/by-uuid)
     * LABEL=name - filesystem label (/dev/disk/by-label)
     * /dev/name - full path to entry in /dev
     * name - short name of device in /dev/ (example: vda)

 * 'path' refers to the path in the filesystem to the soci repository.
   It defaults to 'oci'.

 * 'mdpath': not required, default is '/run/initramfs/oci'.  This will
   be passed as '--metadata-path' to `mos soci mount`.  It is only
   relavant if the type of the layer is squashfs.

 * 'url' refers to a distribution-compliant OCI repository from which
   to fetch the manifest.  if the value is "local", then a local zot
   will be spun up against the backing store under @path on @dev.

## Example
Assuming that you have a disk or partition with a filesystem label 'oci-data'
that contains a top level directory 'oci_repo' with a layer named 'rootfs'.
Create an soci layer rootfs-soci which contains metadata.yaml pointing to
rootfs, manifestCert.pem which is signed by your initrd's manifestCA.pem,
and metadata.yaml.signed signed with the private key for manifestCert.pem.
You can mount that with:

    root=soci:name=rootfs-soci,dev=LABEL=oci-data,path=oci-data

## Notes
 * This module's install does *not* copy in all its dependencies.  This is
   so that those can be added to the initramfs later.  Specifically,
   the root=soci functionality depends on mosctl and zot binaries.

   Those are collected and built into a cpio archive in
   layers/mos/stacker.yaml.
