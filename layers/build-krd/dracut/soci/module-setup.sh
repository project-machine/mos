#!/bin/bash
installkernel() {
    instmods loop squashfs
    return 0
}

check() {
    # we just return 0
    # values (per dracut.modules.7) are:
    #   0: include dracut module in initramfs
    #   1: Do not include module, requirements not met
    # 255: only include the dracut module if requires this.
    return 0
}

depends() {
    # write a list of depends
    :
}

install() {
    inst_hook cmdline 91 "$moddir/soci-cmdline.sh"
    inst_hook initqueue/finished 50 "$moddir/soci-finished.sh"
    inst_hook initqueue/settled 50 "$moddir/soci-settled.sh"
    inst_script "$moddir/soci-lib.sh" /usr/lib/dracut/soci-lib.sh

    # these are required to make LABEL= work well.
    inst "/lib/udev/cdrom_id"
    inst "/lib/udev/rules.d/60-cdrom_id.rules"
    inst mknod
    inst_multiple \
        tpm2_createprimary tpm2_flushcontext tpm2_getcap tpm2_loadexternal \
        tpm2_nvread tpm2_nvreadpublic tpm2_pcrextend tpm2_pcrread \
        tpm2_policyauthorize tpm2_policynv tpm2_policypcr \
        tpm2_startauthsession tpm2_verifysignature tpm2_nvwrite
    inst /usr/lib/x86_64-linux-gnu/libtss2-tcti-device.so.0
    inst curl
    inst git # needed for manifest reading, for now
    #inst /usr/ib/git-core/git-upload-pack
    inst_multiple /usr/lib/git-core/*
    inst chmod
    inst find  # for debug

    return 0
}
