load helpers

function setup() {
	trust_setup
}

function teardown() {
	trust_teardown
}

@test "Keyset creation creates sudi" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust sudi list zomg default
}

@test "Project creation creates sudi" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust project add zomg newproject
	trust sudi list zomg newproject
}

@test "Create sudi" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi
	trust keyset add zomg
	trust project add zomg newproject
	trust sudi add zomg newproject  # auto-create uuid
	trust sudi add zomg newproject 88db65c5-8896-4908-bf8d-8ac04ff20d5c
	[ -e "$MDIR/trust/keys/zomg/manifest/newproject/sudi/88db65c5-8896-4908-bf8d-8ac04ff20d5c/cert.pem" ]
	trust sudi add zomg newproject SN0001
	trust sudi add zomg newproject SN0002
	trust sudi list zomg newproject | grep SN0001
	cnt=$(trust sudi list zomg newproject | wc -l)
	[ $cnt -eq 4 ]
}
