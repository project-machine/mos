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

cp $2 $tmpd/manifest.yaml
cp $3 $tmpd/manifest.yaml.signed
cp $4 $tmpd/manifestCert.pem

cat > $tmpd/stacker.yaml <<EOF
${meta}:
  from:
    type: scratch
  import:
    - path: $tmpd/manifest.yaml
      dest: /
    - path: $tmpd/manifest.yaml.signed
      dest: /
    - path: $tmpd/manifestCert.pem
      dest: /
EOF
stacker --oci-dir ${ocidir} \
	--roots-dir=${tmpd}/roots \
	--stacker-dir=${tmpd}/.stacker \
	build -f ${tmpd}/stacker.yaml \
	--layer-type squashfs

rm -rf $tmpd
