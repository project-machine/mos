load helpers

ROOT_DIR=$(run_git rev-parse --show-toplevel)

function setup() {
	export TMPD=$(mktemp -d "${PWD}/batstest-XXXXX")
	mkdir -p $TMPD/config $TMPD/atomfs $TMPD/scratch
}

function teardown() {
	echo "Deleting $TMPD"
	if [ -n $TMPD ]; then
		rm -rf $TMPD
	fi
}

@test "simple mos install" {
	./mosctl install -c $TMPD/config -a $TMPD/atomfs -s $TMPD/scratch
}
