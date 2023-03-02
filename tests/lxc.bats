# lxc based tests

load helpers

function setup() {
	rm -f setupdone childpid
	start=$(date +%s)
	timeoutsecs=600
	me=$$
	(
		now=$(date +%s)
		d=$(( now - start ))
		echo "now is $now, start is $start"
		while [ ! -f setupdone ] && [ $d -lt $timeoutsecs ]; do
			sleep 5s
			now=$(date +%s)
			d=$(( now - start ))
			echo "now is $now, start is $start"
		done
		[ -f setupdone ] || kill -6 $(<childpid)
	) &
	(
		echo $$ > childpid
		lxc_setup
		echo here i am
		sleep 10
		touch setupdone
	)
}

function teardown() {
	lxc_teardown
}

@test "dummy first lxc job" {
	rm -f done childpid
	start=$(date +%s)
	timeoutsecs=600
	me=$$
	(
		now=$(date +%s)
		d=$(( now - start ))
		echo "now is $now, start is $start"
		while [ ! -f done ] && [ $d -lt $timeoutsecs ]; do
			sleep 5s
			now=$(date +%s)
			d=$(( now - start ))
			echo "now is $now, start is $start"
		done
		[ -f done ] || kill -6 $(<childpid)
	) &
	(
		echo $$ > childpid
		echo Success
		touch done
	)
}
