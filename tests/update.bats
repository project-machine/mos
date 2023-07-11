load helpers

function setup() {
	common_setup
	zot_setup
}

function teardown() {
	zot_teardown
	common_teardown
}

@test "simple mos update from local zot" {
	sum=$(manifest_shasum busybox-squashfs)
	size=$(manifest_size busybox-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": [
    {
      "service_name": "hostfs",
      "version": "1.0.0",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "hostfs",
      "nsgroup": "",
      "network": {
        "type": "host"
      }
    }
  ]
}
EOF
	skopeo copy --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0

	mkdir -p $TMPD/factory/secure
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	[ -f $TMPD/atomfs-store/mos/index.json ]
	sum=$(manifest_shasum busyboxu1-squashfs)
	size=$(manifest_size busyboxu1-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": [
    {
      "service_name": "hostfs",
      "version": "1.0.2",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "hostfs",
      "nsgroup": "",
      "network": {
        "type": "host"
      }
    }
  ]
}
EOF
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	skopeo copy --dest-tls-verify=false oci:zothub:busyboxu1-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	./mosctl update -r $TMPD $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
}

@test "update of fs-only layer" {
	# Simple install
	sum=$(manifest_shasum busybox-squashfs)
	size=$(manifest_size busybox-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": [
    {
      "service_name": "hostfs",
      "version": "1.0.0",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "hostfs",
      "nsgroup": "",
      "network": {
        "type": "host"
      }
    },
    {
      "service_name": "hostfstarget",
      "version": "1.0.0",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "fs-only",
      "nsgroup": "",
      "network": {
        "type": "none"
      }
    }
  ]
}
EOF
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	skopeo copy --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
	# In "real life", /factory/secure/ is set up by the signed initrd
	mkdir -p $TMPD/factory/secure
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	export TMPD
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/factory/secure/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
/bin/ls -l $TMPD/mnt/atom/hostfstarget
cat /proc/self/mountinfo
killall squashfuse || true
XXX
EOF

	# Now upgrade
	sum=$(manifest_shasum busyboxu1-squashfs)
	size=$(manifest_size busyboxu1-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": [
    {
      "service_name": "hostfs",
      "version": "1.0.2",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "hostfs",
      "nsgroup": "",
      "network": {
        "type": "host"
      }
    },
    {
      "service_name": "hostfstarget",
      "version": "1.0.2",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "fs-only",
      "nsgroup": "",
      "network": {
        "type": "none"
      }
    }
  ]
}
EOF
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	skopeo copy --dest-tls-verify=false oci:zothub:busyboxu1-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	echo "BEFORE UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	echo "END OF BEFORE UPDATE"
	./mosctl update -r $TMPD $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2

	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	# And test, making sure the 'u1' file is there
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/factory/secure/manifestCA.pem
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
	sum=$(manifest_shasum busybox-squashfs)
	size=$(manifest_size busybox-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": [
    {
      "service_name": "hostfs",
      "version": "1.0.0",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "hostfs",
      "nsgroup": "",
      "network": {
        "type": "host"
      }
    }
  ]
}
EOF
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	skopeo copy --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
	mkdir -p $TMPD/factory/secure
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0

	# Now do a partial upgrade to install hostfstarget
	sum=$(manifest_shasum busyboxu1-squashfs)
	size=$(manifest_size busyboxu1-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 1,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "partial",
  "targets": [
    {
      "service_name": "hostfstarget",
      "version": "1.0.2",
      "digest": "sha256:$sum",
      "size": $size,
      "service_type": "fs-only",
      "nsgroup": "",
      "network": {
        "type": "none"
      }
    }
  ]
}
EOF
	skopeo copy --dest-tls-verify=false oci:zothub:busyboxu1-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	echo "BEFORE UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status)
	echo "END OF BEFORE UPDATE"
	./mosctl update -r $TMPD $ZOT_HOST:$ZOT_PORT/machine/install:1.0.2
	echo "AFTER UPDATE"
	ls -l $TMPD/config/manifest.git
	(cd $TMPD/config/manifest.git; git status; git log)
	echo "AFTER OF BEFORE UPDATE"

	# Test, make sure the 'u1' file is there in hostfstarget
	lxc-usernsexec -s -- << "EOF"
unshare -m -- << "XXX"
#!/bin/bash
set -e
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/factory/secure/manifestCA.pem
[ -e $TMPD/mnt/atom/hostfstarget/etc ]
/bin/ls -l $TMPD/mnt/atom/hostfstarget
cat /proc/self/mountinfo
# Re-activate, to test stop
./mosctl activate -r $TMPD -t hostfstarget -capath $TMPD/factory/secure/manifestCA.pem
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
./mosctl create-boot-fs --readonly --rfs $TMPD --dest $TMPD/mnt
sleep 1s
[ -e $TMPD/mnt/etc ]
failed=0
echo testing > $TMPD/mnt/helloworld || failed=1
[ $failed -eq 1 ]
killall squashfuse || true
XXX
EOF
}
