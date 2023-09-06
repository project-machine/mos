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
@test "Build and boot provisioning and install ISOs" {
	set -ex
	export ZOT_VERSION=2.0.0-rc5
	cp ${ORIG}/mosctl .
	cp ${ORIG}/trust .
	cp ${ORIG}/tests/livecd2/stacker.yaml .
	cp ${ORIG}/tests/livecd2/mos-install* .
	cp ${ORIG}/tests/livecd2/zot-config.json .
	cp ${ORIG}/tests/livecd2/zot.service .
	stacker --debug build --layer-type=squashfs \
		--stacker-file ${ORIG}/tests/livecd2/stacker.yaml \
		--substitute TMPD=${TMPD} \
		--substitute TOPDIR=${TOPDIR} \
		--substitute KEYSDIR=~/.local/share/machine/trust/keys/snakeoil \
		--substitute "ZOT_VERSION=${ZOT_VERSION}" \
		--substitute "ROOTFS_VERSION=${ROOTFS_VERSION}"
	export PATH=${TMPD}:$PATH
	cp ${ORIG}/tests/livecd2/build-livecd-rfs .
	cd $TMPD
	trust sudi list snakeoil default | grep mosCI001 || trust sudi add snakeoil default mosCI001
	mkdir SUDI; cp ~/.local/share/machine/trust/keys/snakeoil/manifest/default/sudi/mosCI001/* SUDI/
	truncate -s 20M sudi.vfat
	mkfs.vfat -n trust-data sudi.vfat
	mcopy -i sudi.vfat SUDI/cert.pem ::cert.pem
	mcopy -i sudi.vfat SUDI/privkey.pem ::privkey.pem
	qemu-img create -f qcow2 ${TMPD}/livecd.qcow2 120G
	machine init ${VMNAME} << EOF
    name: ${VMNAME}
    type: kvm
    ephemeral: false
    description: A fresh VM booting trust LiveCD in SecureBoot mode with TPM
    config:
      name: ${VMNAME}
      uefi: true
      uefi-vars: $HOME/.local/share/machine/trust/keys/snakeoil/bootkit/ovmf-vars.fd
      cdrom: $HOME/.local/share/machine/trust/keys/snakeoil/artifacts/provision.iso
      boot: cdrom
      tpm: true
      gui: false
      serial: true
      tpm-version: 2.0
      secure-boot: true
      disks:
          - file: ${TMPD}/livecd.qcow2
            type: ssd
            size: 120G
          - file: ${TMPD}/sudi.vfat
            format: raw
            type: hdd
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
	"provisioned successfully" {
		puts "success"
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
	wait_for_vm_down

	# We've provisioned.  Create install iso
	./build-livecd-rfs --layer oci:oci:install-rootfs-squashfs \
		--bootkit-layer oci:oci:bootkit-squashfs \
		--output install.iso --tlayer oci:oci:target-rootfs-squashfs

	mv -f install.iso $HOME/.local/share/machine/trust/keys/snakeoil/artifacts/install.iso
	echo "updating ${VMNAME} to boot from install.iso"
	echo "yaml before:"
	machine info "${VMNAME}"
	cat > sed1.bash << EOF
#!/bin/bash
sed -i 's/provision.iso/install.iso/' \$*
EOF
	chmod 755 sed1.bash
	VISUAL=$(pwd)/sed1.bash machine edit "${VMNAME}"
	export VISUAL=${TOPDIR}/tools/machine_remove_sudi.py
	timeout 10s machine edit "${VMNAME}"
	export -n VISUAL
	machine start "${VMNAME}"
	wait_for_vm
	echo "about to start provisioned machine to install"
	machine info "${VMNAME}"
	sleep 3s

	expect <<EOF
spawn machine console ${VMNAME}
set timeout 120
expect {
	"installed successfully" {
		puts "installed successfully"
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
expect {
	"Power down" { puts "VM powered down after install" }
	timeout {
		puts "timed out"
		exit 1
	}
}
EOF
	wait_for_vm_down

	# we've installed.  Now try to boot
	cat > sed2.bash << EOF
#!/bin/bash
sed -i '/boot:/d;/cdrom:/d' \$*
EOF
	chmod 755 sed2.bash
	VISUAL=$(pwd)/sed2.bash machine edit "${VMNAME}"
	machine start "${VMNAME}"
	wait_for_vm
	machine info "${VMNAME}"
	sleep 3s

	expect <<EOF
spawn machine console ${VMNAME}
set timeout 120
send "\n"
expect {
	"localhost login" { puts "got login prompt" }
	timeout {
		puts "timed out at final boot"
		exit 1
	}
}
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
send "poweroff -f\n"
expect {
       "Power down" { puts "success powering down" }
       timeout {
               puts "timed out waiting for shutdown on final boot"
               exit 1
       }
}
EOF
	wait_for_vm_down
	echo "SUCCESS"
}
