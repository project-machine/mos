load helpers

function setup() {
	common_setup
}

function teardown() {
	echo nah
}

@test "create boot filesystem" {
	good_install

	mkdir -p "${TMPD}/mnt"
	lxc-usernsexec -s -- <<EOF
unshare -m -- ./mosctl create-boot-fs -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --dest $TMPD/mnt
ls -l $TMPD/mnt > xxx
EOF
}
