#!/bin/bash

set -ex
lxc-create -t download -n mos-test -- -d ubuntu -r jammy -a amd64
echo "lxc.mount.entry = /dev/fuse dev/fuse none bind,create=file 0 0" >> ~/.local/share/lxc/mos-test/config
echo "lxc.include = /usr/share/lxc/config/nesting.conf" >>  ~/.local/share/lxc/mos-test/config
echo "lxc.apparmor.allow_nesting = 1" >>   ~/.local/share/lxc/mos-test/config
lxc-start -n mos-test -l trace -o lxc-log.$$
cat lxc-log.$$
lxc-wait --timeout=60 -n mos-test -s RUNNING
sleep 1
lxc-attach -n mos-test -- apt-get update
lxc-attach -n mos-test -- apt-get -y dist-upgrade
lxc-attach -n mos-test -- apt-get -y install software-properties-common
lxc-attach -n mos-test -- add-apt-repository -y ppa:puzzleos/dev

lxc-attach -n mos-test -- apt-get update
lxc-attach -n mos-test -- apt-get -y install squashfuse libsquashfs1 libgpgme11 git lxc1 ubuntu-release-upgrader-core
lxc-stop -n mos-test
