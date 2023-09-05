load helpers

function setup() {
	trust_setup
}

function teardown() {
	trust_teardown
}

@test "Create snakeoil keyset" {
	trust keyset add snakeoil
	[ -d "$MDIR/trust/keys/snakeoil/.git" ]
	trust keyset list | grep snakeoil
}

@test "Create new keysets" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust keyset add --org "My organization" homenet
	cnt=$(trust keyset list | wc -l)
	[ $cnt -eq 2 ]
}
