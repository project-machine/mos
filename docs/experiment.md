# Trying mos by hand

mos is not intended to be used by hand.  In fact, it must be called from the
signed, verified initrd to create the host's root filesystem without any
chance of being tampered with.  However, you can certainly try the install
and boot steps by hand to get a better feel of how it all fits together.

## machine

(machine)[https://github.com/project-machine/machine] is the package which
runs actual hosts (vms, eventually bare hardware or containers as well).
Here we will use it to start a kvm vm running an Ubuntu livecd, with a
software TPM, UEFI secureboot, and our own secureboot signing certificate
enrolled and pre-built uefi variables.  This is used to verify our pre-built
shim and produce a predictable pcr7.

## trust and mos

(trust)[https://github.com/project-machine/trust] is the program which
signs and verifies things.  We will use it during a preliminary boot
in order to create a luks passphrase and host-specific key and certificate,
and store those in the TPM such that only our pre-authorized pcr7 values
can unlock them.

The program built by this present repo, mosctl, will then be used first
to install a busybox based rfs on the virtual drive.

Next, we'll reboot again, and use trust, again, to unlock the variables
we stored in the TPM.  Note - we are pretending here that we were actually
running from signed initrd.  In that case, after reading the variables we
need from TPM, we would extend pcr7 to prevent anyone else reading them.

Finally, we use mosctl again to create a rootfs, which we can chroot or
pivot-root into, and start a containerized service.

## Note on pcr7 and initrd

In this experiment, we are registering the pcr7 value for an ubuntu
livecd.  This allows us to read the TPM variables after login.  In a
real system, the pcr7 values we would authorize would be those for
a very limited kernel+initrd, which we "extend" (change) before leaving
initrd.  Since the initrd was measured as part of the kernel.efi, we
can trust that initrd has not been tampered with.  Whereas in this
example, we obviously cannot have any actual trust in what is running.

# Details

TBD
