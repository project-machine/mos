load helpers
load vmhelpers

function setup() {
	common_setup
	zot_setup
	vm_setup
}

function teardown() {
	vm_teardown
	zot_teardown
	common_teardown
}

@test "trust launch vm" {
	set -x
	# Publish a manifest pointing at an rfs on zothub.io
	# TODO - we do need to include a bootkit layer to set up
	# an ESP.
	cat > "${TMPD}/manifest.yaml" << EOF
version: 1
product: default
update_type: complete
targets:
  - service_name: hostfs
    source: "docker://zothub.io/machine/bootkit/demo-target-rootfs:0.0.4-squashfs"
    version: 1.0.0
    service_type: hostfs
    nsgroup: "none"
    network:
      type: none
  - service_name: zot
    source: "docker://zothub.io/machine/bootkit/demo-zot:0.0.4-squashfs"
    version: 1.0.0
    service_type: container
    nsgroup: "zot"
    network:
      type: simple
      ipv4: 10.0.3.99/24
      ports:
        - host: 80
          container: 5000
  - service_name: bootkit
    source: "oci:$HOME/.local/share/machine/trust/keys/snakeoil/bootkit/oci:bootkit-squashfs"
    version: 1.0.0
    service_type: fs-only
    nsgroup: "none"
    network:
      type: none
EOF

	mosb --debug manifest publish \
	  --project snakeoil:default \
	  --repo 127.0.0.1:${ZOT_PORT} --name machine/install:1.0.0 \
	  "${TMPD}/manifest.yaml"
	trust launch --project=snakeoil:default ${VMNAME} 10.0.2.2:$ZOT_PORT/machine/install:1.0.0
	# Update machine to have network with our port forward
	export VISUAL=${TOPDIR}/tools/machine_add_zotnet.py
	timeout 10s machine edit "${VMNAME}"
	export -n VISUAL
	machine start ${VMNAME}
	wait_for_vm
	expect <<EOF
spawn machine console ${VMNAME}
set timeout 600
expect {
	"localhost login:" { puts "got login prompt" }
	timeout {
		puts "timed out waiting for login prompt"
		exit 1
	}
}
puts "logging in"
send "root\n"
expect {
	"assword" { puts "got password prompt" }
	timeout {
		puts "timed out waiting for password prompt on final boot"
		exit 1
	}
}
send "passw0rd\n"
expect {
	"bash" { puts "got shell" }
	"root@localhost:" { puts "got shell" }
	timeout {
		puts "timed out waiting for shell on final boot"
		exit 1
	}
}
send "lxc-ls -f\n"
expect {
	"bash" { puts "got shell" }
	"root@localhost:" { puts "got shell" }
	timeout {
		puts "timed out waiting for lxc-ls output"
		exit 1
	}
}
send "ip addr\n"
expect {
	"bash" { puts "got shell" }
	"root@localhost:" { puts "got shell" }
	timeout {
		puts "timed out waiting for ip addr output"
		exit 1
	}
}
EOF
	# Check that zot in the installed VM is accessible
	count=1
	while [ $count -lt 100 ]; do
		curl http://127.0.0.1:28080/v2/_catalog && break
		sleep 1s
		count=$((count+1))
	done
	if [ $count -eq 100 ]; then
		echo "launch: FAIL (failed to retrieve zot catalog)"
		false
	fi
	machine delete "${VMNAME}"
	echo "launch: SUCCESS"
}
