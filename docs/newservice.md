# Hooking up a new service

To set up a new service, or services, to run in project machine,
you need to understand the following.

What you'll get in return is the assurance that if the running
instance is corrupted through an online or offline attack, it
will fail to boot or fail to attest its validity (*1).

An example of launching the resulting service can be seen
[here](https://asciinema.org/a/D5otlqvK70BEc6YS49b35HpfY).

## OCI

Your services must be available as [OCI
images](https://github.com/opencontainers/image-spec) using squashfs layers
instead of tar.gz, and with dm-verity root hashes listed as an annotation.  The
easiest way to create such images is using
[stacker](https://github.com/project-stacker/stacker) and building and
publishing using '''--image-type=squashfs'''.

You should use [notation](https://github.com/notaryproject/notation) to sign
your layers.  For now, verify those layers out of band.  In the future, mosb
will consult your notation configuration to verify them in-line.

## Storage

Persistent storage has not yet been implemented.  For now, use
the network, e.g. etcd, nfs, cifs, etc.

## Network

There are currently 3 network options for services:

1. "none": the service cannot reach the network at all
2. "host": the service is in the host's network namespace.  It will not be privileged, so will not be able to change settings, run wireshark, etc.
3. "simple": the service has a [veth](https://man7.org/linux/man-pages/man4/veth.4.html) nic on the lxcbr0 bridge on the host.

The first two support no further configuration, but "simple" has a few options:

```
    network:
      type: simple
      ipv4: 10.0.3.99/24
      ports:
        - host: 80
          container: 5000
```

In the above example, we specify that the IP address for the
service should be 10.0.3.99, and that port 80 on the host should
be forwarded to port 5000 in the container.

For communication between services, they can simply talk to each other
over the ip address you've assigned.

For communication with the outside world which is initiated by the
container, things should "just work" - so long as the provider has
network enabled - since lxcbr0 is nat'd.

For communication with the outside world which is initiated inbound,
you may need to configure the provider.

Currently the only provider is the kvm provider.  By default, this does
not set up a network connection to the outside world.  A configuration
like the following:

```
  nics:
  - bootindex: 'off'
    device: virtio-net
    id: nic0
    network: user
    ports:
    - guest:
        address: ''
        port: 22
      host:
        address: ''
        port: 22222
      protocol: tcp
    romfile: ''
```

will set up a qemu user nic with port 22222 on the host forwarded to
port 22 in the machine.  Note that, in turn, you would need to use
a service 'network' section to forward port 22 on the machine to port
22 (or something else) in the service.

## Footnotes

*1: the attestation service has not yet been implemented.
