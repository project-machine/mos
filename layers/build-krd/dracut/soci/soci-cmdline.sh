#!/bin/bash
# parse the command line, set rootok.

. ${LIB_DRACUT_D:-/usr/lib/dracut}/soci-lib.sh

soci_initrd_start() {
    soci_var_info
    $SOCI_ENABLED || return
    # this sets the global dracut variables 'rootok' and 'root'
    rootok=1
    root=${SOCI_ROOT}

    local dep="" missing=""
    for dep in zot mosctl; do
        command -v "$dep" >/dev/null 2>&1 || missing="$missing, binary '$dep'"
    done

    for dep in /pcr7data/ /manifestCA.pem ; do
        [ -e "$dep" ] || missing="$missing, path '$dep'"
    done

    missing=${missing#, }
    [ -z "$missing" ] || {
        missing=${missing#, }
        soci_die "missing dependencies for 'root=$root': ${missing}"
    }
}

soci_set_vars
soci_initrd_start
