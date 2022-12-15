load helpers

ROOT_DIR=$(run_git rev-parse --show-toplevel)

function setup() {
	export TMPD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p $TMPD/config $TMPD/atomfs
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
	skopeo copy docker://busybox:latest oci:$TMPD/oci:hostfs
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml
	[ -f $TMPD/atomfs/puzzleos/hostfs/index.json ]
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
	mkdir -p $TMPD/zot/c3
	skopeo copy docker://busybox:latest oci:$TMPD/zot/c3/hostfs:2.0.1
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
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml || failed=1
	[ $failed -eq 1 ]
}
