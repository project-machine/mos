function vm_setup() {
	VMNAME="batsvm_$(mktemp -u XXXXXX)"
	export VMNAME

	trust keyset add snakeoil
	# Next step shouldn't be needed once 'trust keyset add' does it for us
	ORIG=$(pwd)
	cd $TMPD
	cp ${ORIG}/mosb .
	cp ${ORIG}/mosctl .
	${ORIG}/tests/livecd1/build-bootkit
}

function vm_teardown() {
	machine delete ${VMNAME}
}

function wait_for_vm() {
	count=0
	while [ $count -lt 5 ]; do
		machine info ${VMNAME}
		s=$(machine info ${VMNAME} | grep "^status:" | awk -F: '{ print $2 }');
		echo "machine status is $s (machine info returned $?)"
		if [ "$s" != " running" ]; then
			break
		fi
		count=$((count+1))
		sleep 1
	done
	if [ $count -ge 5 ]; then
		echo "failed starting test VM"
		exit 1
	fi
}
