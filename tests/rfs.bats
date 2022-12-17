load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "create boot filesystem" {
	good_install

	mkdir -p "${TMPD}/mnt"
	./mosctl create-boot-fs -c $TMPD/config -a $TMPD/atomfs \
		-s $TMPD/scratch-writes --dest $TMPD/mnt
}
