package trust

import (
	"errors"

	"github.com/canonical/go-tpm2"
	"github.com/canonical/go-tpm2/util"
)

// GenLuksPolicy creates a tpm ea policy digest using the pcr7 value
// while booting with uki-production key. This policy is used to
// access the luks secret in the TPM.
// It returns the TPM EA Policy Digest that is generated.
func GenLuksPolicy(prodPcr7 []byte, policyVersion string) ([]byte, error) {
	// Policy Version must be 4 digits. If not given, then use Default.
	if policyVersion == "" {
		policyVersion = PolicyVersion.String()
	} else {
		if len(policyVersion) != 4 {
			return nil, errors.New("Policy version should be a four digit string. i.e. 0001")
		}
		for _, c := range policyVersion {
			if c < '0' || c > '9' {
				return nil, errors.New("Policy version should be four digits. i.e. 0001")
			}
		}
	}

	// Put the pcr7 value in a tpm2.PCRSValues structure so we can compute its digest.
	values := make(tpm2.PCRValues)
	err := values.SetValue(tpm2.HashAlgorithmSHA256, 7, prodPcr7)
	if err != nil {
		return nil, err
	}
	pcrDigest, err := util.ComputePCRDigest(tpm2.HashAlgorithmSHA256, tpm2.PCRSelectionList{{Hash: tpm2.HashAlgorithmSHA256, Select: []int{7}}}, values)
	if err != nil {
		return nil, err
	}

	// Create a tpm2.NVPublic structure that resembles what we would have
	// done via an nvwrite of the policy version to the index.
	// Include TPMA_NV_WRITTEN attribute indicating the index has been written to.
	nvpub := tpm2.NVPublic{Index: tpm2.Handle(TPM2IndexEAVersion), NameAlg: tpm2.HashAlgorithmSHA256, Attrs: tpm2.NVTypeOrdinary.WithAttrs(tpm2.AttrNVOwnerWrite | tpm2.AttrNVOwnerRead | tpm2.AttrNVAuthRead | tpm2.AttrNVWritten), Size: 4}

	trial := util.ComputeAuthPolicy(tpm2.HashAlgorithmSHA256)
	trial.PolicyPCR(pcrDigest, tpm2.PCRSelectionList{{Hash: tpm2.HashAlgorithmSHA256, Select: []int{7}}})
	trial.PolicyNV(nvpub.Name(), []byte(policyVersion), 0, tpm2.OpEq)
	return trial.GetDigest(), nil
}

// GenPasswdPolicy creates a tpm ea policy digest using the pcr7 value
// while booting with uki-tpm key. This policy is used to
// access the tpm password in the TPM.
// It returns the TPM EA Policy Digest that is generated.
func GenPasswdPolicy(tpmPcr7 []byte) ([]byte, error) {
	// Put the pcr7 value in a tpm2.PCRSValues structure so we can compute its digest.
	values := make(tpm2.PCRValues)
	err := values.SetValue(tpm2.HashAlgorithmSHA256, 7, tpmPcr7)
	if err != nil {
		return nil, err
	}
	pcrDigest, err := util.ComputePCRDigest(tpm2.HashAlgorithmSHA256, tpm2.PCRSelectionList{{Hash: tpm2.HashAlgorithmSHA256, Select: []int{7}}}, values)
	if err != nil {
		return nil, err
	}

	// Use a "trial" session to compute the policy digest.
	trial := util.ComputeAuthPolicy(tpm2.HashAlgorithmSHA256)
	trial.PolicyPCR(pcrDigest, tpm2.PCRSelectionList{{Hash: tpm2.HashAlgorithmSHA256, Select: []int{7}}})
	return trial.GetDigest(), nil
}
