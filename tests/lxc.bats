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

@test "activate of fs-only layer in lxc" {
	lxc_install fsonly
	expect << "EOF"
spawn lxc-attach -n mos-test-1
expect "root@mos-test-1"
send "mosctl --debug activate -t hostfstarget\n"
expect "root@mos-test-1"
send "while ! mountpoint -q /mnt/atom/hostfstarget; do sleep 1s; echo -n .; done\n"
expect "root@mos-test-1"
send "touch /mountpointfound\n"
expect "root@mos-test-1"
EOF
	# ugh - give squashfuse just a little *more* time
	sleep 5s
	lxc-attach -n mos-test-1 -- /bin/ls -l /mnt/atom/hostfstarget
	lxc-attach -n mos-test-1 -- test -e /mnt/atom/hostfstarget/etc
	lxc-attach -n mos-test-1 -- cat /proc/self/mountinfo
}

@test "activate of container layer" {
	lxc_install containeronly
	expect << "EOF"
spawn lxc-attach -n mos-test-1
expect "root@mos-test-1"
send "mosctl --debug activate -t hostfstarget\n"
expect "root@mos-test-1"
EOF
	lxc-attach -n mos-test-1 -- lxc-wait -n hostfstarget -s RUNNING -t 0
	pid1=$(lxc-attach -n mos-test-1 -- lxc-info -n hostfstarget -p -H)
	# Re-activate
	# This works on a full system (vm), but fails in a container
	# due to stacker/atomfs dmverity issue (fix in PR).
	expect << "EOF"
spawn lxc-attach -n mos-test-1
expect "root@mos-test-1"
send "mosctl --debug activate -t hostfstarget\n"
expect "root@mos-test-1"
EOF
	lxc-attach -n mos-test-1 -- lxc-wait -n hostfstarget -s RUNNING -t 0
	pid2=$(lxc-attach -n mos-test-1 -- lxc-info -n hostfstarget -p -H)
	test $pid1 -ne $pid2
}
