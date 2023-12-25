# Details of the image manifest

This document will show you how to walk through the full set of
machine configuriaton files, and show you how each piece is
verified.

## Service OCI image

### building the demo-zot layer

For our example service, we are using an instance of the
[zot](https://zothub.io) container image registry.  We build
this image using [stacker](https://stackerbuild.io), using
the following spec:

```
install-base:
  build_only: true
  from:
    type: docker
    url: "docker://zothub.io/machine/bootkit/rootfs:${{ROOTFS_VERSION}}-squashfs"

install-rootfs-pkg:
  build_only: true
  from:
    type: built
    tag: install-base
  run: |
    #!/bin/sh -ex
    pkgtool install \
        cryptsetup \
        dosfstools \
        e2fsprogs \
        efibootmgr \
        iproute2 \
        isc-dhcp-client \
        keyutils \
        kmod \
        libsquashfs-dev \
        parted \
        tpm2-tools \
        udev

    writefile() {
      mkdir -p "${1%/*}"
      echo "write $1" 1>&2
      cat >"$1"
    }

    writefile /etc/systemd/network/20-wire-enp0s-dhcp.network <<"END"
    [Match]
    Name=enp0s*
    [Network]
    DHCP=yes
    END
demo-zot:
  from:
    type: built
    tag: install-rootfs-pkg
  import:
    - zot-config.json
    - start-zot
    - https://github.com/project-zot/zot/releases/download/v${{ZOT_VERSION}}/zot-linux-amd64-minimal
  entrypoint: /usr/bin/start-zot
  run: |
    #!/bin/sh -ex
    cp /stacker/imports/zot-config.json /etc/

    cp /stacker/imports/start-zot /usr/bin/start-zot
    chmod 755 /usr/bin/start-zot
    cp /stacker/imports/zot-linux-amd64-minimal /usr/bin/zot
    chmod 755 /usr/bin/zot

```

and build it using the following command (assuming the above contents are
in the file 'stacker.yaml'):

```
stacker build --layer-type=squashfs  \
    --substitute ROOTFS_VERSION=v0.0.19.231225 \
    --substitute ZOT_VERSION=2.0.0-rc5
```

You can build this and publish to a local zot, but we have already
published one at docker://zothub.io/machine/bootkit/demo-zot:0.0.4-squashfs ,
so will skip that step here.

### Signing

The demo-zot layer should be verified before the next step, but as we are
currently leaving that out of band, we won't address that here right now.

## Manifest yaml

To build an actual bootable machine serving the demo-zot OCI layer, we will
begin with the following yaml file:

```
version: 1
product: default
update_type: complete
targets:
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
```

This specifies the URI to use as the container image.  The 'nsgroup' can
be ignored for now.  If several services have the same nsgroup, then they
will have the same uid mappings, meaning they will be able to have access
to each others files, if they are mapped in.  However, storage mapping is
currently not implemented - doing that is probably the next step.  If the
nsgroup is 'none', then the container will not run in a user namespace, so
it will use the host uid mapping - root will be uid 0 on the host.  This is
not recommended.

The network section specifies that port 80 on the host should be forwarded to
port 5000 in the container.

We will "compile" and sign this using the 'machine os builder' - mosb. To do
that, we need a local zot running:

```
cat > zot-config.json << EOF
{
  "distSpecVersion": "1.1.0-dev",
  "storage": {
    "rootDirectory": "/tmp/zot",
    "gc": false
  },
  "http": {
    "address": "127.0.0.1",
    "port": "5000"
  },
  "log": {
    "level": "debug"
  }
}
EOF
wget -O zot https://github.com/project-zot/zot/releases/download/v2.0.0-rc5/zot-linux-amd64-minimal
chmod 755 zot
./zot serve zot-config.json
```

You'll also need to have created the snakeoil keyset, using the program
'trust' built from this project:

```
./trust keyset add snakeoil
```

Now compile the manifest using:

```
mosb --debug manifest publish \
  --project snakeoil:default \
  --repo 127.0.0.1:5000 --name machine/install:1.0.0 \
  manifest.yaml
```

## Manifest json

The resulting json manifest and signature are created as OCI artifacts in the
zot archive.  Let's take a look.  Assuming that you used the zot config above,
your zot repo is in /tmp/zot.  Since we called the manifest machine/install:1.0.0,
zot will store this as an image called '1.0.0' in the oci layout in directory
machine/install/.  Let's start at the 'index.json':

```
cd /tmp/zot/machine/install
jq . < index.json
{
  "schemaVersion": 2,
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:d63dbe48800f04a141f414e30f2d3b00b61d00e50d4b3ceaf0fc8e7e4953de13",
      "size": 584,
      "annotations": {
        "org.opencontainers.image.ref.name": "1.0.0"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:5fddac82c01188d2389e86dfe41f8293ba6815f041f0f9a196cee275410912a4",
      "size": 755
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:04db4846f9b5b9f9cf54c1f4f718a592b73c0b727b8fb05a40b70e68bf5cf376",
      "size": 765
    }
  ]
}
```

The first manifest, d63be, points to the actual mainfest.json.  We will look
at that later.  First let's look at the other two manifests.  These point at
artifacts containing the signature of manifest.json which mosb created, and
the public key which can be used to verify it.

```
jq . < blobs/sha256/5fddac82c01188d2389e86dfe41f8293ba6815f041f0f9a196cee275410912a4
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.machine.pubkeycrt",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
    "size": 2,
    "data": "e30="
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:136d70873171b931a0e0002fbb31589786038f9419a687f9a47e77b423ba6911",
      "size": 1285,
      "annotations": {
        "org.opencontainers.image.title": "cert.pem"
      }
    }
  ],
  "subject": {
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "digest": "sha256:d63dbe48800f04a141f414e30f2d3b00b61d00e50d4b3ceaf0fc8e7e4953de13",
    "size": 584
  },
  "annotations": {
    "org.opencontainers.image.created": "2023-12-20T08:48:03-06:00"
  }
}
```

This one contains the public key.  You can actually verify that
blobs/sha256/136d70873171b931a0e0002fbb31589786038f9419a687f9a47e77b423ba6911 is
the same file as $HOME/.local/share/machine/trust/keys/snakeoil/manifest/default/cert.pem.
Note that the "subject" "digest" points back at the shasum of the manifest.json
itself

You can actually use the referrers API to query for these, so long
as your zot is still running:

```
curl http://127.0.0.1:5000/v2/machine/install/referrers/sha256:d63dbe48800f04a141f414e30f2d3b00b61d00e50d4b3ceaf0fc8e7e4953de13 | jq .
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:5fddac82c01188d2389e86dfe41f8293ba6815f041f0f9a196cee275410912a4",
      "size": 755,
      "annotations": {
        "org.opencontainers.image.created": "2023-12-20T08:48:03-06:00"
      },
      "artifactType": "application/vnd.machine.pubkeycrt"
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:04db4846f9b5b9f9cf54c1f4f718a592b73c0b727b8fb05a40b70e68bf5cf376",
      "size": 765,
      "annotations": {
        "org.opencontainers.image.created": "2023-12-20T08:48:03-06:00"
      },
      "artifactType": "application/vnd.machine.signature"
    }
  ]
}

```

Now you can manually verify the signature if you like.  mos will do this
itself when installing.

```
openssl dgst -sha256 -verify blobs/sha256/136d70873171b931a0e0002fbb31589786038f9419a687f9a47e77b423ba6911 -signature blobs/sha256/313ac3c232b47ead121c3ed0a3a0e38f9768d57cfa5a4c76649f2d51dc73efd9 blobs/sha256/d63dbe48800f04a141f414e30f2d3b00b61d00e50d4b3ceaf0fc8e7e4953de13
```

Now let's look at the actual machine.json:

```
jq . < blobs/sha256/d63dbe48800f04a141f414e30f2d3b00b61d00e50d4b3ceaf0fc8e7e4953de13
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.machine.install",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
    "size": 2,
    "data": "e30="
  },
  "layers": [
    {
      "mediaType": "application/vnd.machine.install",
      "digest": "sha256:50a93be19bf1027b8363554359e288890324fa7e21002df072e50955e8336954",
      "size": 829,
      "annotations": {
        "org.opencontainers.image.title": "index.json"
      }
    }
  ],
  "annotations": {
    "org.opencontainers.image.created": "2023-12-20T08:48:03-06:00"
  }
}
jq . < blobs/sha256/50a93be19bf1027b8363554359e288890324fa7e21002df072e50955e8336954
{
  "version": 1,
  "product": "default",
  "targets": [
    {
      "service_name": "zot",
      "version": "1.0.0",
      "service_type": "container",
      "network": {
        "type": "simple",
        "ipv4": "10.0.3.99/24",
        "ipv6": "",
        "ports": [
          {
            "host": 80,
            "container": 5000
          }
        ]
      },
      "nsgroup": "zot",
      "digest": "sha256:3f7fe7839527f97a351e3e7790cfa42f85ff89a8b5dfab699812feeddd1c7c04",
      "size": 7290
    },
    {
      "service_name": "hostfs",
      "version": "v0.0.19.231225",
      "service_type": "hostfs",
      "network": {
        "type": "host",
        "ipv4": "",
        "ipv6": "",
        "ports": null
      },
      "nsgroup": "",
      "digest": "sha256:95c77628b5ccd1f74fd4538f367800c18a371321731408a29428153b1893d9f9",
      "size": 7288
    },
    {
      "service_name": "bootkit",
      "version": "1.0.0",
      "service_type": "fs-only",
      "network": {
        "type": "host",
        "ipv4": "",
        "ipv6": "",
        "ports": null
      },
      "nsgroup": "",
      "digest": "sha256:83f822a13bac8b61b17b36d582030f38c75f631629866d2b02be49514baa5145",
      "size": 1637
    }
  ],
  "update_type": "complete"
}
```

You can see that apart from switching from a more human-readable yaml to
a more reproducible json file, the manifest.json also adds two layers.  The
bootkit layer comes from your trust keyset, and contains a signed shim and
signed UKI (unified kernel image).  The shim contains the keys for verifying
the UKI.  The UKI contains a kernel and initramfs.  The hostfs layer has
a minimal init and the mosctl program to launch your container services.
If you want or need to specify a custom hostfs, you can do so by specifying
one in the manifest.yaml that you feed to 'mosb manifest publish'.

## Machine configuration

Project machine intends to support a variety of platforms: kvm,
incus, bare hardware, etc - anything with a TPMv2 can be a substrate.
Each substrate will be abstracted as a "provider".  For now, only the
kvm provider, which uses [machine](https://github.com/project-machine/machine)
to launch local kvm virtual machines, is implemented.

To launch a machine running this manifest, run:

```
./trust launch --project=snakeoil:default vm1 10.0.2.2:5000/machine/install:1.0.0
```

This will create the VM and boot it twice: once from the provisioning ISO
($HOME/.local/share/machine/trust/keys/snakeloil/artifacts/provision.iso), and
once from the install iso ($HOME/.local/share/machine/trust/keys/snakeloil/artifacts/install.iso)
passing the URL to install from (10.0.2.2:5000/machine/install:1.0.0).

Once the VM is ready, you can see its definition using:


```
# machine list
NAME      STATUS   DESCRIPTION
----      ------   -----------
vm1       stopped  A fresh VM booting trust LiveCD in SecureBoot mode with TPM
# serge@serge-l-PF3DENS3 /tmp/zot/machine/install$ machine info vm1
type: kvm
config:
  name: vm1
  cpus: 2
  memory: 2048
  serial: "true"
  nics: []
  disks:
  - file: /home/serge/.local/state/machine/machines/vm1/vm1/vm1.qcow2
    size: 120000000000
    type: ssd
  boot: hdd
  cdrom: ""
  uefi-code: /home/serge/.local/share/machine/trust/keys/snakeoil/bootkit/ovmf/ovmf-code.fd
  uefi-vars: /home/serge/.local/share/machine/trust/keys/snakeoil/bootkit/ovmf-vars.fd
  tpm: true
  tpm-version: "2.0"
  secure-boot: true
  gui: true
description: A fresh VM booting trust LiveCD in SecureBoot mode with TPM
ephemeral: false
name: vm1
status: stopped
```

The file defining this is in $HOME/.config/machine/machines/vm1/machine.yaml.
The 'nics' field is empty, meaning that the VM will not have any network
interfaces.  In the future this wlil be automatically configured to expose
the ports you specified in the service configurations (manifest.yaml), but
for now that is un-implemented, so you must change the configuration yourself,
for instance:

```
  nics:
  - bootindex: 'off'
    device: virtio-net
    id: nic0
    network: user
    ports:
    - guest:
        address: ''
        port: 80
      host:
        address: ''
        port: 28080
      protocol: tcp
    romfile: ''
```

After this, your vm is ready to use:

```
machine start vm1
machine console vm1
# log in as root/passw0rd
```

From another terminal, you can interact with zot:

```
curl http://127.0.0.1:28080/v2/
```

as demonstrated [here](https://asciinema.org/a/D5otlqvK70BEc6YS49b35HpfY).
