#!/bin/bash

set -x

# Set up a host to be ready for building and testing project-machine
mkdir -p ~/bin

sudo apt-get update
sudo add-apt-repository -y ppa:puzzleos/dev
sudo apt-get -y install \
	bats cryptsetup-bin expect libacl1-dev libarchive-tools \
	libcryptsetup-dev libgpgme-dev libcap-dev \
	libdevmapper-dev liblxc-dev libpam0g-dev \
	libseccomp-dev libsquashfs-dev lxc lxc-dev make mtools\
	openssl pip pkgconf skopeo socat squashfuse swtpm jq \
	uidmap umoci qemu-utils qemu-system-x86 xorriso \
	ubuntu-dev-tools make gcc squashfs-tools sbsigntool \
	python3-yaml
sudo modprobe kvm
sudo adduser $(whoami) kvm
sudo chmod o+rw /dev/kvm
sudo systemctl restart user@$(id -u runner)
sudo systemctl start dbus
sudo pip install virt-firmware

#wget -O ~/bin/stacker --progress=dot:mega https://github.com/project-stacker/stacker/releases/download/v1.0.0-rc6/stacker
wget -O ~/bin/stacker --progress=dot:mega http://hallyn.com:55589/stacker
chmod 755 ~/bin/stacker
which stacker
whereis stacker
stacker --version

### DELME - this is for testing only
mkdir -p /home/runner/actions-runner/_work/_tool/stacker/1.0.0-rc6/x64
cp ~/bin/stacker /home/runner/actions-runner/_work/_tool/stacker/1.0.0-rc6/x64/stacker
### end DELME

wget -O ~/bin/skopeo --progress=dot:mega https://github.com/project-machine/tools/releases/download/v0.0.1/skopeo
chmod 755 ~/bin/skopeo
sudo cp -f ~/bin/skopeo /usr/bin/skopeo

wget -O ~/bin/machine --progress=dot:mega https://github.com/project-machine/machine/releases/download/v0.1.2/machine-linux-amd64
wget -O ~/bin/machined --progress=dot:mega https://github.com/project-machine/machine/releases/download/v0.1.2/machined-linux-amd64
chmod 755 ~/bin/machine ~/bin/machined
mkdir -p ~/.config/systemd/user/
export  PATH=~/bin:$PATH

mkdir -p /run/user/$(id -u)/containers
chmod go+rx /run/user/$(id -u)
chmod go+rx /run/user/$(id -u)/containers

chmod ugo+x $HOME
cat /etc/subuid /etc/subgid
u=$(id -un)
g=$(id -gn)
echo "u=$u g=$g"
uidmap=$(awk -F: '$1 == u { print $2, $3 }' "u=$u" /etc/subuid)
gidmap=$(awk -F: '$1 == g { print $2, $3 }' "g=$g" /etc/subgid)
if [ "$u" = "runner" ] && [ -z "$gidmap" ]; then
	# 'id -gn' shows docker, but 'runner' is in subgid
	g="runner"
	gidmap=$(awk -F: '$1 == g { print $2, $3 }' "g=$g" /etc/subgid)
fi
echo "uidmap=$uidmap."
echo "gidmap=$gidmap."
[ -n "$uidmap" ] && [ -n "$gidmap" ] || \
	{ echo "did not get uidmap or gidmap for u=$u g=$g"; exit 1; }
mkdir -p ~/.config/lxc/
tee ~/.config/lxc/default.conf <<EOF
lxc.include = /etc/lxc/default.conf
lxc.idmap = u 0 $uidmap
lxc.idmap = g 0 $gidmap
EOF

echo "$u veth lxcbr0 100" | sudo tee -a /etc/lxc/lxc-usernet
