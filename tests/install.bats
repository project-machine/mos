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

@test "simple mos install" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
targets:
  - layer: docker://zothub.io/c3/ubuntu/base-amd64:jammy
    name: hostfs
    service_type: hostfs
    nsgroup: ""
    mounts: []
EOF
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -f $TMPD/install.yaml
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
