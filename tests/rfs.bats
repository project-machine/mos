load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "create ro boot filesystem" {
	good_install

	mkdir -p "${TMPD}/mnt"
	lxc-usernsexec -s -- <<EOF
unshare -m -- <<XXX
./mosctl create-boot-fs --readonly -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --dest $TMPD/mnt
sleep 1s
[ -e $TMPD/mnt/etc ]
failed=0
echo testing > $TMPD/mnt/helloworld || failed=1
[ $failed -eq 1 ]
killall squashfuse
XXX
EOF
}

@test "create rw boot filesystem" {
	good_install

	mkdir -p "${TMPD}/mnt"
	lxc-usernsexec -s -- <<EOF
unshare -m -- <<XXX
./mosctl create-boot-fs -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --dest $TMPD/mnt
sleep 1s
[ -e $TMPD/mnt/etc ]
echo testing > $TMPD/mnt/helloworld
[ -f $TMPD/mnt/helloworld ]
killall squashfuse
XXX
EOF
}
