load helpers

function setup() {
	trust_setup
}

function teardown() {
	trust_teardown
}

@test "Create keysets, project and sudis" {
	if [ "$(arch)" != "x86_64" ]; then
		skip "Not supported on $(arch)"
	fi

	# Create new project in snakeoil keyset
	trust keyset list | grep snakeoil || trust keyset add snakeoil
	trust project add snakeoil newproject
	trust project list snakeoil | grep newproject
	cnt=$(trust project list snakeoil | wc -l)
	[ $cnt -eq 2 ]

	# Create new keyset and new project
	trust keyset add --org "Wowza inc" zomg
	cnt=$(trust keyset list | wc -l)
	[ $cnt -eq 2 ]

	# Create new project in that new keyset
	trust project list zomg | grep default
	trust project add zomg newproject
	trust project list zomg | grep newproject
	cnt=$(trust project list zomg | wc -l)
	[ $cnt -eq 2 ]

	# Sudi list should succeed without any sudis
	trust sudi list zomg default
	trust sudi list zomg newproject
	cnt=$(trust sudi list zomg newproject | wc -l)
	[ $cnt -eq 0 ]

	# Create new sudis
	trust sudi add zomg newproject  # auto-create uuid
	trust sudi add zomg newproject 88db65c5-8896-4908-bf8d-8ac04ff20d5c
	[ -e "$MDIR/trust/keys/zomg/manifest/newproject/sudi/88db65c5-8896-4908-bf8d-8ac04ff20d5c/cert.pem" ]
	trust sudi add zomg newproject SN0001
	trust sudi add zomg newproject SN0002
	trust sudi list zomg newproject | grep SN0001
	cnt=$(trust sudi list zomg newproject | wc -l)
	[ $cnt -eq 4 ]
}
