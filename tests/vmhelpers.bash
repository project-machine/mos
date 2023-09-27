function vm_setup() {
	VMNAME="batsvm_$(mktemp -u XXXXXX)"
	export VMNAME

	ORIG=$(pwd)
	cd $TMPD
	cp ${ORIG}/mosb .
	cp ${ORIG}/mosctl .
}

function vm_teardown() {
	machine delete ${VMNAME}
}

function wait_for_vm() {
	count=0
	while [ $count -lt 5 ]; do
		s=$(machine info ${VMNAME} | awk -F\  '/status:/ { print $2 }')
		if [ "$s" = "running" ]; then
			break
		fi
		echo "machine status is $s (machine info returned $?)"
		count=$((count+1))
		sleep 1
	done
	if [ $count -ge 5 ]; then
		echo "failed starting test VM"
		exit 1
	fi
	echo "machine is up"
}

function wait_for_vm_down() {
	count=0
	maxcount=20
	while [ $count -lt $maxcount ]; do
		s=$(machine info ${VMNAME} | awk -F\  '/status:/ { print $2 }')
		if [ "$s" = "stopped" ]; then
			break
		fi
		if [ "$s" = "failed" ]; then
			echo "Warning: ${VMNAME} status is \"failed\""
			break
		fi
		echo "machine info returned status code $?. machine status is $s."
		count=$((count+1))
		sleep 1
	done
	if [ $count -ge $maxcount ]; then
		echo "failed waiting for test VM to stop"
		exit 1
	fi
}
