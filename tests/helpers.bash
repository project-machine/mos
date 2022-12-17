function run_git {
    git "$@"
}

function common_setup {
	if [ ! -d "${PWD}/zothub" ]; then
		stacker --oci-dir zothub build --layer-type squashfs
	fi

	if [ -z "${KEYS_DIR}" ]; then
		export KEYS_DIR="${PWD}/keys"
		if [ ! -d "${KEYS_DIR}" ]; then
			git clone https://github.com/project-machine/keys
		fi
	fi
	export TMPD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p "$TMPD/config" "$TMPD/atomfs" "$TMPD/scratch-writes"
	# TODO I'm using the ca cert bc we don't have a sample manifest signing cert yet.
	# switch that over when it's available.
	cp "${KEYS_DIR}/sampleproject/manifest.crt" "$TMPD/manifestCert.pem"
}

function common_teardown {
	echo "Deleting $TMPD"
	if [ -n $TMPD ]; then
		lxc-usernsexec -s -- rm -rf $TMPD
	fi
}

function good_install {
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
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	cp "${KEYS_DIR}/manifestCA/cert.pem" "$TMPD/config/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml
}
