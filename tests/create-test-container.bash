#!/bin/bash

set -ex
lxc-create -t download -n mos-test -- -d ubuntu -r jammy -a amd64
echo "lxc.mount.entry = /dev/fuse dev/fuse none bind,create=file 0 0" >> ~/.local/share/lxc/mos-test/config
echo "lxc.include = /usr/share/lxc/config/nesting.conf" >>  ~/.local/share/lxc/mos-test/config
echo "lxc.apparmor.allow_nesting = 1" >>   ~/.local/share/lxc/mos-test/config
lxc-start -n mos-test -l trace -o lxc-log.$$ || { echo "lxc-start mos-test failed"; cat lxc-log.$$; exit 1; }
lxc-wait --timeout=60 -n mos-test -s RUNNING || { echo "lxc-wait mos-test failed"; cat lxc-log.$$; exit 1; }
echo $?
cat /etc/lxc/default.conf
cat ~/.local/share/lxc/mos-test/config
cat lxc-log.$$
ifconfig -a
lxc-attach -n mos-test -- ip link
lxc-attach -n mos-test -- ip addr
lxc-attach -n mos-test -- ip route
lxc-attach -n mos-test -- ps -ef
sudo dmesg
ps -ef
count=0
lxc-attach -n mos-test -- << EOF
while [ $count -lt 10 ]; do
  if apt-get update; then
    break
  fi
  sleep 1s
  count=$((count+1))
done
EOF
lxc-attach -n mos-test -- apt-get -y dist-upgrade
lxc-attach -n mos-test -- apt-get -y install software-properties-common
lxc-attach -n mos-test -- add-apt-repository -y ppa:puzzleos/dev

lxc-attach -n mos-test -- apt-get update
lxc-attach -n mos-test -- apt-get -y install squashfuse libsquashfs1 libgpgme11 git lxc1 ubuntu-release-upgrader-core
lxc-stop -n mos-test
