# lxc based tests

load helpers

function setup() {
	lxc_setup
}

function teardown() {
	lxc_teardown
}

# This is to test the test infrastructure itself.  If this fails,
# then lxc is not set up correctly.
@test "install of simple system in an lxc container" {
	lxc_install hostfsonly
}
