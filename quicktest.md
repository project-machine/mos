# Notes for testing mosctl in a vm or bare metal

sudo apt install libsquashfs1, lxc1, skopeo
NOTES - need newer skopeo in jammy!

sudo mkdir -p /config /atomfs-store /scratch-writes
# Can't have overlay as basis for workdir/upperdir
sudo mount -t tmpfs tmpfs /scratch-writes

mkdir target
cat > target/install.yaml << EOF
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - layer: docker://zothub.local/c3/hostfs:2.0.1
    name: hostfs
    fullname: puzzleos/hostfs
    version: 1.0.0
    service_type: hostfs
    nsgroup: ""
    network:
      type: host
    mounts: []
  - layer: docker://zothub.local/c3/hostfs:2.0.1
    name: fstarget
    fullname: puzzleos/fstarget
    version: 1.0.0
    service_type: fs-only
    nsgroup: ""
    network:
      type: none
    mounts: []
  - layer: docker://zothub.local/c3/hostfs:2.0.1
    name: ctarget
    fullname: puzzleos/ctarget
    version: 1.0.0
    service_type: container
    nsgroup: c1
    network:
      type: host
    mounts: []
EOF

export TMPD=target
openssl dgst -sha256 -sign "keys/sampleproject/manifest.key" \
	-out "$TMPD/install.yaml.signed" "$TMPD/install.yaml"
skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:hostfs
skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:fstarget
skopeo copy oci:zothub:busybox-squashfs oci:$TMPD/oci:ctarget
cp keys/manifestCA/cert.pem target/manifestCA.pem
sudo mkdir -p /factory/secure
sudo cp keys/manifestCA/cert.pem /factory/secure/manifestCA.pem
cp keys/sampleproject/manifest.crt target/manifestCert.pem

sudo chmod go+x /var/lib/lxc  # should we do this in mos?  Or expect a proper rfs?
#sudo ./mostctl install -f target/install.yaml
#sudo ./mostctl activate -t fstarget; ls /mnt/atom/fstarget
#sudo ./mostctl activate -t ctarget; sudo lxc-ls -f
#sudo mkdir /sysroot; sudo ./mosctl create-boot-fs; ls /sysroot
