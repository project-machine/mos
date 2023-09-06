#!/bin/bash
# root=soci:<string>
#  <string> is comma separated tokens
#    dev=(LABEL|UUID|DEV|ID|NAME)=<value>
#    path=<value> - path to oci repo within dev - default is 'oci'
#    name=<value> - name of layer to use.

if ! command -v getarg >/dev/null; then
    . ${LIB_DRACUT_D:-/usr/lib/dracut}/../dracut-lib.sh
fi

SOCI_DEBUG="true"
msg() {
    local lvl="$1" con=false pre="" up="" idle=""
    shift
    pre="[$lvl]"
    case "$lvl" in
        ERROR|WARN|INFO) con="true";;
        *) [ "$SOCI_DEBUG" = "true" ] && con=true;;
    esac
    read up idle </proc/uptime
    echo "$pre" "[${up}s]" "$@" >> "${SOCI_LOG_D}/initrd.log"
    [ "$con" = "false" ] || printf "%s\n" "$pre $*" >/dev/console
}

soci_error() { msg ERROR "$@"; }
soci_info() { msg INFO "$@"; }
soci_debug() { msg DEBUG "$@"; }
soci_warn() { msg WARN "$@"; }
soci_die() {
    local msg="SOCI root boot failed."
    if ! getargbool 0 rd.shell; then
        msg="$msg Boot with rd.shell to debug."
    fi
    [ $# -eq 0 ] || soci_error "$@"

    die "$msg"
}

soci_log_run() {
    local fname="$SOCI_LOG_D/run.log" ret="" out="" quiet=false
    if [ "$1" == "--quiet" ]; then
        quiet=true
        shift
    fi
    soci_debug "running $*"
    {
        echo "$" "$*"
        out=$("$@" 2>&1)
        ret=$?
        echo "$out"
        echo ":: $ret"
    } >> "$fname" 2>&1
    if [ $ret -ne 0 -a "$quiet" = "false" ]; then
        soci_error "Failed [$ret] running: $*"
        soci_error "$out"
    else
        [ "$SOCI_DEBUG" = "true" -a -n "$out" ] && soci_debug "$out"
        soci_debug "returned $ret"
    fi
    return $ret
}

safe_string() {
    local prev="$1" allowed="$2" cur=""
    [ -n "$prev" ] || return 1
    while cur="${prev#[${allowed}]}"; do
        [ -z "$cur" ] && return 0
        [ "$cur" = "$prev" ] && break
        prev="$cur"
    done
    return 1
}

parse_string() {
    # parse a key/value string like:
    # name=mapper,pass=foo,fstype=ext4,mkfs=1
    # set variables under namespace 'ns'.
    #  _RET_name=mapper
    #  _RET_pass=foo
    #  _RET_fstype=ext4
    # set _RET to the list of variables found
    local input="${1}" delim="${2:-,}" ns="${3:-_RET_}"
    local oifs="$IFS" tok="" keys="" key="" val=""
    set -f; IFS="$delim"; set -- $input; IFS="$oifs"; set +f;
    _RET=""
    for tok in "$@"; do
        key="${tok%%=*}"
        val="${tok#${key}}"
        val=${val#=}
        safe_string "$key" "0-9a-zA-Z_" ||
            { soci_debug "$key not a safe variable name"; return 1; }
        eval "${ns}${key}"='${val}' || return 1
        keys="${keys} ${ns}${key}"
    done
    _RET=${keys# }
    return
}

soci_set_vars() {
    [ -n "$SOCI_CMDLINE" ] && return 0
    local cmdline="" tok="" _root=""
    read cmdline </proc/cmdline
    SOCI_LOG_D="/run/initramfs/soci"
    [ -d "$SOCI_LOG_D" ] || mkdir -p "$SOCI_LOG_D"
    SOCI_CMDLINE="$cmdline"
    SOCI_FINISHED_MARK="$SOCI_LOG_D/finished"
    for tok in $cmdline; do
        case "$tok" in
            root=*) _root=${tok#root=};;
            rd.soci-debug) SOCI_DEBUG=true;;
        esac
    done
    SOCI_ENABLED=true
    case "$_root" in
        soci:*) SOCI_ROOT="$_root";;
        *) SOCI_ENABLED=false;;
    esac

    [ "$SOCI_ENABLED" = "true" ] || return 0

    # parse_string sets SOCI_ variables from the command line.
    # variables like SOCI_path, SOCI_dev, SOCI_name
    parse_string "${_root#soci:}" , SOCI_
    [ -n "$SOCI_name" ] || {
        soci_die "root=soci: requires 'name' parameter"
        return 1
    }

    [ "${SOCI_path-_unset}" = "_unset" ] && SOCI_path="oci"
}

soci_var_info(){
    if $SOCI_ENABLED; then
        soci_info "soci root. dev=${SOCI_dev} path=${SOCI_path} name=${SOCI_name}"
    else
        soci_info "soci root cmdline not found: $SOCI_CMDLINE"
    fi
}
