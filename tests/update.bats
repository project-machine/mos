load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "simple mos update from local zot" {
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	mkdir -p $TMPD/zot/c3
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
	[ -f $TMPD/atomfs-store/puzzleos/hostfs/index.json ]
	cat > $TMPUD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.2
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	skopeo copy oci:zothub:busyboxu1-squashfs oci:$TMPUD/oci:hostfs
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPUD/install.yaml.signed" "$TMPUD/install.yaml"
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPUD/manifestCA.pem"
	mkdir -p $TMPD/factory/secure
	mkdir -p $TMPD/root
	cp ${KEYS_DIR}/manifest/cert.pem $TMPD/factory/secure/manifestCA.pem
	./mosctl update -r $TMPD -f $TMPUD/install.yaml
}

@test "update of fs-only layer" {
	# Simple install
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
  - service_name: hostfstarget
    zotpath: puzzleos/hostfstarget
    version: 1.0.0
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
    mounts: []
EOF
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	mkdir -p $TMPD/zot/c3
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfstarget
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
/bin/ls -l $TMPD/mnt/atom/hostfstarget
cat /proc/self/mountinfo
killall squashfuse || true
XXX
EOF

	# Now upgrade
	cat > $TMPUD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.2
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
  - service_name: hostfstarget
    zotpath: puzzleos/hostfstarget
    version: 1.0.2
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
    mounts: []
EOF
	skopeo copy oci:zothub:busyboxu1-squashfs oci:$TMPUD/oci:hostfs
	skopeo copy oci:zothub:busyboxu1-squashfs oci:$TMPUD/oci:hostfstarget
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPUD/install.yaml.signed" "$TMPUD/install.yaml"
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPUD/manifestCA.pem"
	mkdir -p $TMPD/factory/secure
	mkdir -p $TMPD/root
	cp ${KEYS_DIR}/manifest/cert.pem $TMPD/factory/secure/manifestCA.pem
	echo "BEFORE UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	echo "END OF BEFORE UPDATE"
	./mosctl update -r $TMPD -f $TMPUD/install.yaml

	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	# And test, making sure the 'u1' file is there
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
/bin/ls -l $TMPD/mnt/atom/hostfstarget
cat /proc/self/mountinfo
[ -e $TMPD/mnt/atom/hostfstarget/u1 ]
killall squashfuse || true
XXX
EOF
}

@test "test partial update" {
	# Simple install
	cat > $TMPD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    zotpath: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
EOF
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
	mkdir -p $TMPD/zot/c3
	skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPD/manifestCA.pem"
	./mosctl install -c $TMPD/config -a $TMPD/atomfs-store -f $TMPD/install.yaml

	# Now do a partial upgrade to install hostfstarget
	cat > $TMPUD/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: partial
targets:
  - service_name: hostfstarget
    zotpath: puzzleos/hostfstarget
    version: 1.0.2
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
    mounts: []
EOF
	skopeo copy oci:zothub:busyboxu1-squashfs oci:$TMPUD/oci:hostfstarget
	openssl dgst -sha256 -sign "${KEYS_DIR}/manifest/privkey.pem" \
		-out "$TMPUD/install.yaml.signed" "$TMPUD/install.yaml"
	cp "${KEYS_DIR}/manifest-ca/cert.pem" "$TMPUD/manifestCA.pem"
	mkdir -p $TMPD/factory/secure
	mkdir -p $TMPD/root
	cp ${KEYS_DIR}/manifest/cert.pem $TMPD/factory/secure/manifestCA.pem
	echo "BEFORE UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	echo "END OF BEFORE UPDATE"
	./mosctl update -r $TMPD -f $TMPUD/install.yaml
	echo "AFTER UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status; git log)
	echo "AFTER OF BEFORE UPDATE"

	# Test, make sure the 'u1' file is there in hostfstarget
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
[ -e $TMPD/mnt/atom/hostfstarget/u1 ]
killall squashfuse || true
XXX
EOF

	# Also make sure we can still mount the hostfs
	mkdir -p "${TMPD}/mnt"
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl create-boot-fs --readonly -c $TMPD/config -a $TMPD/atomfs-store \
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
