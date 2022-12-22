#!/bin/bash

# make-soci.bash
# Given:
#  arg 1: oci dir
#  arg 2: manifest.yaml
#  arg 3: manifest.yaml.signed
#  arg 4: manifestCert.pem
#  arg 5: meta layer name
# Create a new layer ocidir:meta which contains the
# manifest.yaml, manifest.yaml.signed, and manifestCert.pem.
# It's assumed that manifest.yaml lists targets whose layers
# are already in ocidir.

tmpd=$(mktemp -d)
ocidir=$1
meta=$5
umoci new --image ${ocidir}:${meta}
umoci unpack --rootless --image  ${ocidir}:${meta} ${tmpd}
cp $2 $tmpd/rootfs/manifest.yaml
cp $3 $tmpd/rootfs/manifest.yaml.signed
cp $4 $tmpd/rootfs/manifestCert.pem
umoci repack --image ${ocidir}:${meta} $tmpd
