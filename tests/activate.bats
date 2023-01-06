load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "activate of fs-only layer" {
	good_install fsonly
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
/bin/ls -l $TMPD/mnt/atom/hostfstarget
cat /proc/self/mountinfo
# Re-activate, to test stop
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
killall squashfuse || true
XXX
EOF
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
