load helpers

function setup() {
	common_setup
}

function teardown() {
	common_teardown
}

@test "Generate a policy" {
	trust tpm-policy-gen --passwd-pcr7-file sample1/pcr7-tpm.bin \
	    --production-pcr7-file sample1/pcr7-prod.bin \
	    --passwd-policy-file sample1/passwd.out \
	    --luks-policy-file sample1/luks.out
	diff sample1/passwd.out sample1/passwd.policy
	diff sample1/luks.out sample1/luks.policy
}
