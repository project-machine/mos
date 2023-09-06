#!/bin/bash
. ${LIB_DRACUT_D:-/usr/lib/dracut}/soci-lib.sh

soci_check_finished() {
    ${SOCI_ENABLED} || return 0
    [ -e "${SOCI_FINISHED_MARK}" ] || {
        soci_debug "not finished: no soci-finished."
        return 1
    }

    soci_debug "All finished."
    return 0
}

soci_set_vars
soci_check_finished
