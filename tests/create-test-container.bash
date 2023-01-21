#!/bin/bash

lxc-create -t download -n mos-test -- -d ubuntu -r jammy -a amd64
echo "lxc.mount.entry = /dev/fuse dev/fuse none bind,create=file 0 0" >> ~/.local/share/lxc/mos-test/config
echo "lxc.include = /usr/share/lxc/config/nesting.conf" >>  ~/.local/share/lxc/mos-test/config
echo "lxc.apparmor.allow_nesting = 1" >>   ~/.local/share/lxc/mos-test/config
lxc-start -n mos-test
lxc-wait -n mos-test -s RUNNING
lxc-attach -n mos-test -- apt-get update
lxc-attach -n mos-test -- apt-get -y dist-upgrade
lxc-attach -n mos-test -- apt-get -y install software-properties-common

expect << "EOF"
spawn lxc-attach -n mos-test
expect "root@mos-test"
send "add-apt-repository ppa:puzzleos/dev\n"
expect "ENTER"
send "\n"
expect "root@mos-test"
EOF

lxc-attach -n mos-test -- apt-get update
lxc-attach -n mos-test -- apt-get -y install squashfuse libsquashfs1 libgpgme11 git lxc1 ubuntu-release-upgrader-core
lxc-stop -n mos-test
