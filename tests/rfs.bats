load helpers

function setup() {
	common_setup
	zot_setup
}

function teardown() {
	zot_teardown
	common_teardown
}

@test "create ro boot filesystem" {
	good_install hostfsonly

	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl create-boot-fs --readonly --rfs "$TMPD" --dest $TMPD/mnt
sleep 1s
[ -e $TMPD/mnt/etc ]
failed=0
echo testing > $TMPD/mnt/helloworld || failed=1
[ $failed -eq 1 ]
killall squashfuse || true
XXX
EOF
}

@test "create rw boot filesystem" {
	good_install hostfsonly

	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl create-boot-fs --rfs "$TMPD" --dest $TMPD/mnt
sleep 1s
[ -e $TMPD/mnt/etc ]
echo testing > $TMPD/mnt/helloworld
[ -f $TMPD/mnt/helloworld ]
killall squashfuse || true
XXX
EOF
}

@test "create boot filesystem with corrupted manifest" {
	good_install hostfsonly
	# Make the install.json fail verification with install.json.signature
	pushd "${TMPD}/config/manifest.git"
	cat *.json
	sed -i 's/$/ $/' *.json
	git add *.json
	me="test-user <test-user@example.com>"
	git commit -m "corrupt"
	popd

	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
failed=0
if ./mosctl create-boot-fs --rfs "$TMPD" --dest $TMPD/mnt; then
   echo "mosctl create-boot-fs should have failed"
   false
else
   echo "mosctl failed as it should"
   failed=1
fi
[ $failed -eq 1 ]
XXX
EOF
}
