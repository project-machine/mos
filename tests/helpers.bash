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
	export TMPUD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p "$TMPD/config" "$TMPD/atomfs-store" "$TMPD/scratch-writes"
	cp "${KEYS_DIR}/manifest/cert.pem" "$TMPD/manifestCert.pem"
	cp "${KEYS_DIR}/manifest/cert.pem" "$TMPUD/manifestCert.pem"
}

function lxc_setup {
	common_setup
	lxc-info -q -n mos-test || {
		./tests/create-test-container.bash
	}
	lxc-info -q -n mos-test-1 && {
		lxc-destroy -n mos-test-1 -f
	}
	lxc-copy -n mos-test -N mos-test-1
	lxc-start -n mos-test-1
	lxc-wait -n mos-test-1 -s RUNNING
}

function common_teardown {
	echo "Deleting $TMPD and $TMPUD"
	if [ -n $TMPD ]; then
		lxc-usernsexec -s -- rm -rf $TMPD
	fi
	if [ -n $TMPUD ]; then
		lxc-usernsexec -s -- rm -rf $TMPUD
	fi
}

function lxc_teardown {
	#lxc-destroy -n mos-test-1 -f
	common_teardown
}

function manifest_shasum_from {
	target=$1
	jsonindex=$2
	shasum=$(jq '.manifests[] | select(.annotations == {"org.opencontainers.image.ref.name": "'"$target"'"}).digest' $jsonindex)
	echo $shasum | cut -d ':' -f 2 | cut -d "\"" -f 1
}

function manifest_shasum {
	target=$1
	manifest_shasum_from $target zothub/index.json
}

function write_install_yaml {
	spectype=$1
	case "$spectype" in
	  hostfsonly)
	    sum=$(manifest_shasum busybox-squashfs)
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
      type: none
    mounts: []
EOF
	  ;;

	  fsonly)
	    sum=$(manifest_shasum busybox-squashfs)
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
  - service_name: hostfstarget
    zotpath: puzzleos/hostfstarget
    version: 1.0.0
    manifest_hash: $sum
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
    mounts: []
EOF
	    ;;

	  containeronly)
	    sum=$(manifest_shasum busybox-squashfs)
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
  - service_name: hostfstarget
    zotpath: puzzleos/hostfstarget
    version: 1.0.0
    manifest_hash: $sum
    service_type: container
    nsgroup: c1
    network:
      type: host
    mounts: []
EOF
	    ;;
	  *)
	    echo "Test failure: Unknown install yaml type $spectype"
	    false
	    ;;
	esac
}

function good_install {
	spectype=$1
	write_install_yaml "$spectype"
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfstarget
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
}

function lxc_install {
	# set up the file we need under TMPD
	spectype=$1
	write_install_yaml "$spectype"
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfstarget
	cp mosctl ${TMPD}/
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	# copy TMPD over to the container under /iso/
	lxc-attach -n mos-test-1 -- mkdir -p /iso /config /atomfs-store /scratch-writes /factory/secure
	tar -C $TMPD -cf - . | lxc-attach -n mos-test-1 -- tar -C /iso -xf -
	# do the install
	lxc-attach -n mos-test-1 -- cp /iso/manifestCA.pem /factory/secure/
	lxc-attach -n mos-test-1 -- cp /iso/mosctl /usr/bin/
	lxc-attach -n mos-test-1 -- mosctl install -f /iso/install.yaml
}
