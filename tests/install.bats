load helpers

function setup() {
  common_setup
  zot_setup
}

function teardown() {
  zot_teardown
  common_teardown
}

@test "mosctl manifest publish" {
	write_install_yaml "hostfsonly"
	./mosb manifest publish --product snakeoil:default \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name machine/install:1.0.0 \
		$TMPD/manifest.yaml
	[ -f $TMPD/zot/mos/index.json ]  # the layers were pushed
	[ -f $TMPD/zot/machine/install/index.json ]  # the manifest was pushed
  regctl artifact tree $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
}

@test "mosctl manifest publish twice" {
	write_install_yaml "hostfsonly"
	./mosb manifest publish --product snakeoil:default \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name machine/install:1.0.0 \
		$TMPD/manifest.yaml
	./mosb manifest publish --product snakeoil:default \
		--repo ${ZOT_HOST}:${ZOT_PORT} --name machine/install:1.0.0 \
		$TMPD/manifest.yaml
	[ -f $TMPD/zot/mos/index.json ]  # the layers were pushed
	[ -f $TMPD/zot/machine/install/index.json ]  # the manifest was pushed
  regctl artifact tree $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
}

@test "simple mos install from local zot" {
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
	mkdir -p "$TMPD/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs $TMPD $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	[ -f $TMPD/atomfs-store/mos/index.json ]
}

@test "simple mos manifest publish and mos install" {
	good_install hostfsonly
}

@test "simple mos install with bad signature" {
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
	echo "fooled ya" > "$TMPD/install.json.signed"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	mkdir -p "$TMPD/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	failed=0
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 || failed=1
	[ $failed -eq 1 ]
}

@test "mos install with bad version" {
	sum=$(manifest_shasum busybox-squashfs)
	size=$(manifest_size busybox-squashfs)
	cat > $TMPD/install.json << EOF
{
  "version": 2,
  "product": "de6c82c5-2e01-4c92-949b-a6545d30fc06",
  "update_type": "complete",
  "targets": []
}
EOF
	skopeo copy --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
  regctl artifact put --artifact-type application/vnd.machine.install -f "$TMPD/install.json" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
	openssl dgst -sha256 -sign "$M_KEY" \
		-out "$TMPD/install.json.signed" "$TMPD/install.json"
  regctl artifact put --artifact-type application/vnd.machine.pubkeycrt -f "$M_CERT" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0
  regctl artifact put --artifact-type application/vnd.machine.signature -f "$TMPD/install.json.signed" --subject $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0

	failed=0
	mkdir -p "$TMPD/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 || failed=1
	[ $failed -eq 1 ]
}

@test "simple mos install with bad manifest hash" {
	sum=$(manifest_shasum busybox-squashfs)
	skopeo copy --dest-tls-verify=false oci:zothub:busybox-squashfs docker://$ZOT_HOST:$ZOT_PORT/mos:$sum
	size=$(manifest_size busybox-squashfs)
	# Next line is where we make the manifest hash invalid
	sum=$(echo $sum | sha256sum | cut -f 1 -d \ )
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
      "nsgroup": "none",
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

	failed=0
	mkdir -p "$TMPD/factory/secure"
	cp "$CA_PEM" "$TMPD/factory/secure/manifestCA.pem"
	./mosctl install --rfs "$TMPD" $ZOT_HOST:$ZOT_PORT/machine/install:1.0.0 || failed=1
	[ $failed -eq 1 ]
}
