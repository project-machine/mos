# Input file formats: a RFC

A running mos system will use an install.json, which prettified would look
like:

```
{
  version: 1,
  product: de6c82c5-2e01-4c92-949b-a6545d30fc06
  update_type: complete
  targets: [
    {
      service_name: hostfs,
      version: 1.0.0,
      digest: sha256:53fb9924c10ff56f1ae1306e7c70272ea6bdad44ea68ab341ead5ba1f42f51fb,
      size: 645,
      service_type: hostfs,
      nsgroup: none,
      network: {
        type: none
      }
    },
    {
      service_name: ran,
      version: 1.0.0
      digest: sha256:1e2ba5c7c2b12368c550cd5d1bbf8265e4643b78f9d0c07008b1b7e95aeafa42,
      size: 891,
      service_type: container,
      nsgroup: ran,
      network: {
        type: none
      }
    }
  ]
}
```

But in real life, it'll of course have whitespace removed.  That's
ok, only the machine needs to love it.

To build this install.json, the user will write a manifest.yaml
and run it through mosb manifest publish.  The file looks like
this:

```
version: 1
product: de6c82c5-2e01-4c92-949b-a6545d30fc06
update_type: complete
targets:
  - service_name: hostfs
    source: docker://ubuntu:latest
    version: 1.0.0
    digest: sha256:53fb9924c10ff56f1ae1306e7c70272ea6bdad44ea68ab341ead5ba1f42f51fb  # optional
    size: 645
    service_type: hostfs
    nsgroup: none
    network:
      type: none
  - service_name: hostfs
    source: oci:/oci:ran:2.5
    version: 1.0.0
    service_type: hostfs
    nsgroup: ran
    network:
      type: none
```

The digest and size are optional here.  We highly recommend specifying
them when possible, however you can also use cosign to provide the
security guarantees you need.  To accomodate that, if digest
is not provided, then mosb will fill it in in the resulting install.json.

(Note: we are not (yet) enforcing that you cosign-verify each image
that does not have a digest listed here.)

To convert, sign and publish the install manifest, you would run:

```
# mosb manifest publish --file=manifest.yaml \
  --cert=cert.pem --key=privkey.pem \
  --repo=10.3.1.25:5000 --path=machine/install:1.0.0
```

This will convert the manifest.yaml to an install.json, sign it
with privkey.pem, publish the install.json as an artifact to
docker://10.3.1.25:5000/machine/install:1.0.0, and also post
two more artifacts which each refer to the install.json manifest:
one containing the signature, and one containing the cert.pem.
