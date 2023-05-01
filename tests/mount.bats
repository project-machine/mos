load helpers

function setup() {
	common_setup
	zot_setup
}

function teardown() {
	zot_teardown
	common_teardown
}

@test "mount hostfs filesystem" {
	good_install hostfsonly

	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -ex
./mosctl --debug mount --rfs $TMPD --readonly --dest ${TMPD}/mnt
sleep 1s
ls -l $TMPD/mnt
[ -e $TMPD/mnt/etc ]
killall squashfuse || true
XXX
EOF
}

@test "mount ro livecd filesystem" {
	write_install_yaml "livecd"

	./mosb manifest publish --product snakeoil:default \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name machine/livecd:1.0.0 \
		$TMPD/manifest.yaml

	mkdir -p "${TMPD}/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -ex
./mosctl --debug mount --rfs $TMPD --readonly --target livecd --dest ${TMPD}/mnt \
    ${ZOT_HOST}:${ZOT_PORT}/machine/livecd:1.0.0
sleep 1s
ls -l $TMPD/mnt
[ -e $TMPD/mnt/etc ]
failed=0
echo testing > $TMPD/mnt/helloworld || failed=1
[ $failed -eq 1 ]
killall squashfuse || true
XXX
EOF
}

@test "mount rw livecd filesystem" {
	write_install_yaml "livecd"

	./mosb manifest publish --product snakeoil:default \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name machine/livecd:1.0.0 \
		$TMPD/manifest.yaml

	mkdir -p "${TMPD}/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -ex
./mosctl --debug mount --rfs $TMPD --target livecd --dest ${TMPD}/mnt \
    ${ZOT_HOST}:${ZOT_PORT}/machine/livecd:1.0.0
sleep 1s
ls -l $TMPD/mnt
[ -e $TMPD/mnt/etc ]
echo testing > $TMPD/mnt/helloworld
[ -f $TMPD/mnt/helloworld ]
killall squashfuse || true
XXX
EOF
}
