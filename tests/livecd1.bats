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

# 'machine' is expected to be in $PATH, and 'machined' is
# expected to be running.
@test "Build and boot a live-vm" {
	set -ex
	stacker --debug build --layer-type=squashfs \
		--stacker-file ${ORIG}/tests/livecd1/livecd-stacker.yaml \
		--substitute TMPD=${TMPD} \
		--substitute BOOTKIT_VERSION=${BOOTKIT_VERSION}
	export PATH=${TMPD}:$PATH
	cp ${ORIG}/tests/livecd1/build-livecd-rfs .
	cp ${ORIG}/tests/livecd1/hello* .
	./build-livecd-rfs
	qemu-img create -f qcow2 ${TMPD}/livecd.qcow2 20G
	echo "created ${TMPD}/livecd.qcow2"
	machine init ${VMNAME} << EOF
    name: ${VMNAME}
    type: kvm
    ephemeral: false
    description: A fresh VM booting trust LiveCD in SecureBoot mode with TPM
    config:
      name: ${VMNAME}
      uefi: true
      uefi-vars: $HOME/.local/share/machine/trust/keys/snakeoil/bootkit/ovmf-vars.fd
      cdrom: livecd.iso
      boot: cdrom
      tpm: true
      gui: false
      serial: true
      tpm-version: 2.0
      secure-boot: true
      disks:
          - file: ${TMPD}/livecd.qcow2
            type: ssd
            size: 20G
EOF
	machine info ${VMNAME}
	machine start ${VMNAME}
	wait_for_vm
	machine info ${VMNAME}
	sleep 3s

	expect <<EOF
spawn machine console ${VMNAME}
set timeout 120
expect {
	"hello, world: success" {
		puts "success"
		exit 0
	}
	"XXX FAIL XXX" {
		puts "${VMNAME} failed"
		exit 1
	}
	timeout {
		puts "timed out"
		exit 1
	}

}
EOF
}
