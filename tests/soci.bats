load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "make an soci image" {
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
}
