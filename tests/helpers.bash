function run_git {
    git "$@"
}

function trust_setup {
	MDIR=~/.local/share/machine
	BACKUP=~/.local/share/machine.backup
	export TOPDIR="$(git rev-parse --show-toplevel)"
	export PATH=${TOP_DIR}:$PATH
	if [ -d "$BACKUP" ]; then
		rm -rf "$BACKUP"
	fi
	if [ -d "$MDIR" ]; then
		mv "$MDIR" "$BACKUP"
	fi
}

function common_setup {
	trust_setup

	export BOOTKIT_VERSION="${BOOTKIT_VERSION:-v0.0.15.230901}"
	echo "BOOTKIT_VERSION is ${BOOTKIT_VERSION}"

	if [ ! -d "${PWD}/zothub" ]; then
		stacker --oci-dir zothub build --layer-type squashfs
	fi

	# set up test git
	local name="test-user" email="test-user@example.com"
	export \
        GIT_AUTHOR_NAME="$name" \
        GIT_AUTHOR_EMAIL="$email" \
        GIT_COMMITTER_NAME="$name" \
        GIT_COMMITTER_EMAIL="$email"

	# set up temporary directories for installs
	export TMPD=$(mktemp -d "${PWD}/batstest-XXXXX")
	export TMPUD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p "$TMPD/config" "$TMPD/atomfs-store" "$TMPD/scratch-writes" "$TMPD/bin"

	export PATH="$PATH:${TOPDIR}/hack/tools/bin"

	trust keyset list | grep snakeoil || trust keyset add --bootkit-version=${BOOTKIT_VERSION} snakeoil
	export CA_PEM=~/.local/share/machine/trust/keys/snakeoil/manifest-ca/cert.pem
	export M_CERT=~/.local/share/machine/trust/keys/snakeoil/manifest/default/cert.pem
	export M_KEY=~/.local/share/machine/trust/keys/snakeoil/manifest/default/privkey.pem
}

function zot_setup {
	export ZOT_HOST=127.0.0.1
	export ZOT_PORT=5000
	cat > $TMPD/zot-config.json << EOF
{
  "distSpecVersion": "1.1.0-dev",
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
	pid=$!
	# wait until service is up
	count=5
	up=0
	while [[ $count -gt 0 ]]; do
		if [ ! -d /proc/$pid ]; then
			echo "zot failed to start or died"
			exit 1
		fi
		up=1
		curl -f http://$ZOT_HOST:$ZOT_PORT/v2/ || up=0
		if [ $up -eq 1 ]; then break; fi
		sleep 1
		count=$((count - 1))
	done
	if [ $up -eq 0 ]; then
		echo "Timed out waiting for zot"
		exit 1
	fi
  # setup a OCI client
  regctl registry set --tls=disabled $ZOT_HOST:$ZOT_PORT
}

function trust_teardown {
	if [ -d "$MDIR" ]; then
		rm -rf "$MDIR"
	fi
	if [ -d "$BACKUP" ]; then
		mv "$BACKUP" "$MDIR"
	fi
}

function common_teardown {
	echo "Deleting $TMPD and $TMPUD"
	if [ -n $TMPD ]; then
		lxc-usernsexec -s -- rm -rf $TMPD
	fi
	if [ -n $TMPUD ]; then
		lxc-usernsexec -s -- rm -rf $TMPUD
	fi
	trust_teardown
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

function manifest_size_from {
	target=$1
	jsonindex=$2
	size=$(jq '.manifests[] | select(.annotations == {"org.opencontainers.image.ref.name": "'"$target"'"}).size' $jsonindex)
	echo $size
}

function manifest_size {
	target=$1
	manifest_size_from $target zothub/index.json
}

function write_install_yaml {
	spectype=$1
	sum=$(manifest_shasum busybox-squashfs)
	size=$(manifest_size busybox-squashfs)
	case "$spectype" in
	  livecd)
	    cat > $TMPD/manifest.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: livecd
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
EOF
	  ;;

	  hostfsonly)
	    cat > $TMPD/manifest.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: hostfs
    nsgroup: ""
    network:
      type: none
EOF
	  ;;

	  fsonly)
	    sum=$(manifest_shasum busybox-squashfs)
	    cat > $TMPD/manifest.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
  - service_name: hostfstarget
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
EOF
	    ;;

	  containeronly)
	    sum=$(manifest_shasum busybox-squashfs)
	    cat > $TMPD/manifest.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
  - service_name: hostfstarget
    source: oci:zothub:busybox-squashfs
    version: 1.0.0
    digest: sha256:$sum
    size: $size
    service_type: container
    nsgroup: c1
    network:
      type: host
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
	./mosb manifest publish \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name puzzleos/install:1.0.0 \
		--project snakeoil:default $TMPD/manifest.yaml
	rm $TMPD/manifest.yaml
	mkdir -p $TMPD/factory/secure
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl --debug install --rfs "$TMPD" \
	    ${ZOT_HOST}:${ZOT_PORT}/puzzleos/install:1.0.0
}
