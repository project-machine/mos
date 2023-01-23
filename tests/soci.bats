load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "make and mount an soci image" {
	echo -n "soci target fs" > $TMPD/IWASHERE
	cat > $TMPD/stacker.yaml << EOF
hostfs:
  from:
    type: oci
    url: zothub:busybox-squashfs
  import:
    - path: ${TMPD}/IWASHERE
      dest: /
EOF
	stacker --oci-dir $TMPD/oci --roots-dir=${TMPD}/roots \
	  --stacker-dir=${TMPD}/.stacker \
	  build -f ${TMPD}/stacker.yaml \
	  --layer-type squashfs
	umoci tag --image ${TMPD}/oci:hostfs-squashfs hostfs

	sum=$(manifest_shasum_from hostfs $TMPD/oci/index.json)
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	cat $TMPD/install.yaml
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	mkdir -p $TMPD/oci/puzzleos/hostfs
	# the referenced sOCI layer must be in zot, not simple oci layout
	skopeo copy oci:${TMPD}/oci:hostfs oci:$TMPD/oci/puzzleos/hostfs:1.0.0

	./tests/make-soci-layer.bash $TMPD/oci $TMPD/install.yaml \
		$TMPD/install.yaml.signed \
		$TMPD/manifestCert.pem \
		hostfs-meta
	umoci ls --layout $TMPD/oci | grep hostfs-meta

	mkdir ${TMPD}/mnt
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl soci mount --ocidir ${TMPD}/oci  \
    --metalayer hostfs-meta-squashfs \
    --capath ${KEYS_DIR}/manifest-ca/cert.pem \
    --mountpoint ${TMPD}/mnt
diff ${TMPD}/mnt/IWASHERE ${TMPD}/IWASHERE
XXX
EOF
}

@test "soci image with bad manifest hash" {
	echo -n "soci target fs" > $TMPD/IWASHERE
	cat > $TMPD/stacker.yaml << EOF
hostfs:
  from:
    type: oci
    url: zothub:busybox-squashfs
  import:
    - path: ${TMPD}/IWASHERE
      dest: /
EOF
	stacker --oci-dir $TMPD/oci --roots-dir=${TMPD}/roots \
	  --stacker-dir=${TMPD}/.stacker \
	  build -f ${TMPD}/stacker.yaml \
	  --layer-type squashfs
	umoci tag --image ${TMPD}/oci:hostfs-squashfs hostfs

	sum=$(manifest_shasum_from hostfs $TMPD/oci/index.json)
	sum=$(echo $sum | sha256sum | cut -f 1 -d \ )
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	mkdir -p $TMPD/oci/puzzleos/hostfs
	skopeo copy oci:${TMPD}/oci:hostfs oci:$TMPD/oci/puzzleos/hostfs:1.0.0

	./tests/make-soci-layer.bash $TMPD/oci $TMPD/install.yaml \
		$TMPD/install.yaml.signed \
		$TMPD/manifestCert.pem \
		hostfs-meta
	umoci ls --layout $TMPD/oci | grep hostfs-meta

	mkdir ${TMPD}/mnt
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
failed=0
./mosctl soci mount --ocidir ${TMPD}/oci  \
    --metalayer hostfs-meta-squashfs \
    --capath ${KEYS_DIR}/manifest-ca/cert.pem \
    --mountpoint ${TMPD}/mnt || failed=1
[ $failed -eq 1 ]
XXX
EOF
}
