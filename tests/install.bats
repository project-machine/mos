load helpers

ROOT_DIR=$(run_git rev-parse --show-toplevel)

function setup() {
	if [ -z "${KEYS_DIR}" ]; then
		export KEYS_DIR="${PWD}/keys"
		if [ ! -d "${KEYS_DIR}" ]; then
			git clone https://github.com/project-machine/keys
		fi
	fi
	export TMPD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p "$TMPD/config" "$TMPD/atomfs"
	# TODO I'm using the ca cert bc we don't have a sample manifest signing cert yet.
	# switch that over when it's available.
	cp "${KEYS_DIR}/sampleproject/manifest.crt" "$TMPD/manifestCert.pem"
}

function teardown() {
	echo "Deleting $TMPD"
	if [ -n $TMPD ]; then
		rm -rf $TMPD
	fi
}

@test "simple mos install from local oci" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
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
	skopeo copy docker://busybox:latest oci:$TMPD/oci:hostfs
	cp "${KEYS_DIR}/manifestCA/cert.pem" "$TMPD/config/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml
	[ -f $TMPD/atomfs/puzzleos/hostfs/index.json ]
	[ -f $TMPD/config/manifest.git/manifest.yaml ]
}

@test "simple mos install with bad signature" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
targets:
  - layer: docker://zothub.local/c3/hostfs:2.0.1
    name: hostfs
    fullname: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    mounts: []
EOF
	echo "fooled ya" > "$TMPD/install.yaml.signed"
	skopeo copy docker://busybox:latest oci:$TMPD/oci:hostfs
	failed=0
	cp "${KEYS_DIR}/manifestCA/cert.pem" "$TMPD/config/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}

@test "simple mos install from local zot" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
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
	mkdir -p $TMPD/zot/c3
	skopeo copy docker://busybox:latest oci:$TMPD/zot/c3/hostfs:2.0.1
	cp "${KEYS_DIR}/manifestCA/cert.pem" "$TMPD/config/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml
	[ -f $TMPD/atomfs/puzzleos/hostfs/index.json ]
}

@test "mos install with bad version" {
	cat > $TMPD/install.yaml << EOF
version: 2
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
targets:
EOF
	failed=0
	cp "${KEYS_DIR}/manifestCA/cert.pem" "$TMPD/config/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}
