function run_git {
    git "$@"
}

function common_setup {
	if [ ! -d "${PWD}/zothub" ]; then
		stacker --oci-dir zothub build --layer-type squashfs
	fi

	local name="test-user" email="test-user@example.com"
	export \
        GIT_AUTHOR_NAME="$name" \
        GIT_AUTHOR_EMAIL="$email" \
        GIT_COMMITTER_NAME="$name" \
        GIT_COMMITTER_EMAIL="$email"

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
	lxc-start -n mos-test-1 -l trace -o xxx || true
	cat xxx
	lxc-start -n mos-test-1
	lxc-wait --timeout=60 -n mos-test-1 -s RUNNING
}

function zot_setup {
  export ZOT_HOST=127.0.0.1
  export ZOT_PORT=5000
  cat > $TMPD/zot-config.json << EOF
{
  "distSpecVersion": "1.0.1-dev",
  "storage": {
    "rootDirectory": "$TMPD/zot",
    "gc": false
  },
  "http": {
    "address": "$ZOT_HOST",
    "port": "$ZOT_PORT"
  },
  "log": {
    "level": "error"
  }
}
EOF
  # start as a background task
  zot serve $TMPD/zot-config.json &
  # wait until service is up
  while true; do x=0; curl -f http://$ZOT_HOST:$ZOT_PORT/v2/ || x=1; if [ $x -eq 0 ]; then break; fi; sleep 1; done
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

function zot_teardown {
  killall zot
  rm -f $TMPD/zot-config.json
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
	pathtype=$1
	spectype=$2
	case "$spectype" in
	  hostfsonly)
	    sum=$(manifest_shasum busybox-squashfs)
	    if [ "$pathtype" = "ocipath" ]; then
	      imagepath=oci:zothub:busybox-squashfs
	    else
	      imagepath=puzzleos/hostfs
	    fi
	    cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: ${imagepath}
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
	    if [ "$pathtype" = "ocipath" ]; then
	      imagepath=oci:zothub:busybox-squashfs
	    else
	      imagepath=puzzleos/hostfs
	    fi
	    cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: ${imagepath}
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
  - service_name: hostfstarget
    imagepath: ${imagepath}
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
	    if [ "$pathtype" = "ocipath" ]; then
	      imagepath=oci:zothub:busybox-squashfs
	    else
	      imagepath=puzzleos/hostfs
	    fi
	    cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    imagepath: ${imagepath}
    version: 1.0.0
    manifest_hash: $sum
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
  - service_name: hostfstarget
    imagepath: ${imagepath}
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
	write_install_yaml ocipath "$spectype"
	./mosb iso build --key "${KEYS_DIR}/manifest/privkey.pem" \
		--cert "${KEYS_DIR}/manifest-ca/cert.pem" \
		--file $TMPD/install.yaml \
		--output-file $TMPD/mos.iso
	rm $TMPD/install.yaml
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	# Just expand the iso in $TMPD
	(pushd $TMPD; bsdtar -x -f mos.iso; rm -f mos.iso; popd)
	./mosctl --debug install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
}

function lxc_install {
	# set up the file we need under TMPD
	spectype=$1
	write_install_yaml zotpath "$spectype"
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
