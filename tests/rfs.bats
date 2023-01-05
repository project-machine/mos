load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "create ro boot filesystem" {
	good_install hostfsonly

	mkdir -p "${TMPD}/mnt"
	lxc-usernsexec -s -- <<EOF
unshare -m -- <<XXX
./mosctl create-boot-fs --readonly -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --ca-path $TMPD/manifestCA.pem --dest $TMPD/mnt
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
	lxc-usernsexec -s -- <<EOF
unshare -m -- <<XXX
./mosctl create-boot-fs -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --ca-path $TMPD/manifestCA.pem --dest $TMPD/mnt
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
	# Make the install.yaml fail verification with install.yaml.signature
	pushd "${TMPD}/config/manifest.git"
	sed -i 's/^targets:/\n\0/' *.yaml
	git add *.yaml
	git commit -m "corrupt"
	popd

	mkdir -p "${TMPD}/mnt"
	lxc-usernsexec -s -- <<EOF
unshare -m -- <<XXX
failed=0
if ./mosctl create-boot-fs -c $TMPD/config -a $TMPD/atomfs \
   -s $TMPD/scratch-writes --ca-path $TMPD/manifestCA.pem --dest $TMPD/mnt; then
   echo "mosctl create-boot-fs should have failed"
   false
else
   echo "mosctl failed as it should"
   failed=1
fi
XXX
EOF
}
