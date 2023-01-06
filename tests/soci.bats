load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "make and mount an soci image" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - layer: docker://zothub.local/c3/hostfs:2.0.1
    name: hostfs
    fullname: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	openssl dgst -sha256 -sign "${KEYS_DIR}/sampleproject/manifest.key" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs

	./tests/make-soci-layer.bash $TMPD/oci $TMPD/install.yaml \
		$TMPD/install.yaml.signed \
		$TMPD/manifestCert.pem \
		hostfs-meta
	umoci ls --layout $TMPD/oci | grep hostfs-meta

	echo -n "soci target fs" > $TMPD/IWASHERE
	cat > $TMPD/stacker.yaml << EOF
hostfs:
  from:
    type: oci
    url: $TMPD/oci:hostfs
  import:
    - path: ${TMPD}/IWASHERE
      dest: /
EOF
	stacker --oci-dir $TMPD/oci --roots-dir=${TMPD}/roots \
	  --stacker-dir=${TMPD}/.stacker \
	  build -f ${TMPD}/stacker.yaml \
	  --layer-type squashfs
	umoci tag --image ${TMPD}/oci:hostfs-squashfs hostfs
	mkdir ${TMPD}/mnt
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl soci mount --ocidir ${TMPD}/oci  \
    --metalayer hostfs-meta-squashfs \
    --capath ${KEYS_DIR}/manifestCA/cert.pem \
    --mountpoint ${TMPD}/mnt
diff ${TMPD}/mnt/IWASHERE ${TMPD}/IWASHERE
XXX
EOF
}
