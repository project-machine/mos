# layers

## provision

The provision-rootfs layer can be used to provision a new
machine using 'trust'.

## install

The install-rootfs layer can be used to install a provisioned
machine using mosctl.  There is also a demo-target-rootfs
layer.  This can be used as a layer to install on a machine,
and will result in a booting system with a debug shell on
serial.  This is obviously unsafe and only to be used for
demo purposes with throwaway keysets.

## usage

You shouldn't need to deal with these directly:  trust keyset
add will create a provisioning ISO using the rootfs-provision
layer.  The trust keyset or project add command could create
an install ISO, although the install script would first need
to be augmented to get a manifest distoci url for the system
to install.
