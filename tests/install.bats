load helpers

function setup() {
  common_setup
  zot_setup
}

function teardown() {
  zot_teardown
  common_teardown
}

@test "simple mos install from local oci" {
	good_install hostfsonly
	cat $TMPD/install.yaml
	[ -f $TMPD/atomfs-store/busybox-squashfs/index.json ]
	[ -f $TMPD/config/manifest.git/manifest.yaml ]
}

@test "simple mos install with bad signature" {
	sum=$(manifest_shasum busybox-squashfs)
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: puzzleos/hostfs
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	echo "fooled ya" > "$TMPD/install.yaml.signed"
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	failed=0
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}

@test "simple mos install from local zot" {
	sum=$(manifest_shasum busybox-squashfs)
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: puzzleos/hostfs
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
	mkdir -p $TMPD/zot/c3
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
	[ -f $TMPD/atomfs-store/puzzleos/hostfs/index.json ]
}

@test "simple mos install from local zot with references" {
	sum=$(manifest_shasum busybox-squashfs)
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: puzzleos/hostfs
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
  mkdir -p $TMPD/zot/c3
  skopeo copy --format=oci --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/oci:hostfs
  skopeo copy --src-tls-verify=false docker://$ZOT_HOST:$ZOT_PORT/oci:hostfs oci:$TMPD/oci:hostfs
  # push the install manifest
  oras push --plain-http --image-spec v1.1-image $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 "$TMPD/install.yaml":vnd.machine.install
  # make a copy of the cert, which we will push and restore
  mkdir -p "$TMPD/manifest-ca"
  cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifest-ca"
  # make this a cert a reference to the install manifest and remove the local copy
  oras attach --plain-http --image-spec v1.1-image --artifact-type vnd.machine.public-key $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 "$TMPD/manifest-ca/cert.pem"
  rm "$TMPD/manifest-ca/cert.pem"
  # ensure this association is made on the OCI registry
  oras discover --plain-http -o json --artifact-type vnd.machine.public-key $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  PUBKEY=$(oras discover --plain-http -o json --artifact-type vnd.machine.public-key $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 | jq .manifests[0].digest | tr -d \")
  # restore the cert copy from the OCI registry
  oras pull --plain-http -o "$TMPD/manifest-ca/" $ZOT_HOST:$ZOT_PORT/machine/install@$PUBKEY
  cp "$TMPD/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
  ./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
  [ -f $TMPD/atomfs-store/puzzleos/hostfs/index.json ]
}

@test "mos install with bad version" {
	sum=$(manifest_shasum busybox-squashfs)
	cat > $TMPD/install.yaml << EOF
version: 2
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
EOF
	failed=0
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}

@test "simple mos install with bad manifest hash" {
	sum=$(manifest_shasum busybox-squashfs)
	sum=$(echo $sum | sha256sum | cut -f 1 -d \ )
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: puzzleos/hostfs
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	failed=0
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}

