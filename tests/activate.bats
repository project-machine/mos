load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "activate of fs-only layer" {
	good_install fsonly
}

# TODO - right now this is not implemented (needs the lxc container
# setup, shiftfs, and systemd service unit setup), so expected to fail
@test "activate of container layer" {
	good_install containeronly
}

# TODO - right now this is not implemented (reboot), so expected to fail
@test "activate of hostfs layer" {
	good_install hostfsonly
}
