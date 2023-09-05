load helpers

function setup() {
	trust_setup
}

function teardown() {
	trust_teardown
}

@test "Keyset creation creates default project" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust project list zomg | grep default
}

@test "Create project" {
	trust keyset add snakeoil
	trust project add snakeoil newproject
	trust project list snakeoil | grep newproject
	cnt=$(trust project list snakeoil | wc -l)
	[ $cnt -eq 2 ]
}

@test "Create project in custom keyset" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust project add zomg newproject
	trust project list zomg | grep newproject
	cnt=$(trust project list zomg | wc -l)
	[ $cnt -eq 2 ]
}
