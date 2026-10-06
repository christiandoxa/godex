//go:build !linux

package superexpose

type systemSessionProcessInspector struct{}

func (systemSessionProcessInspector) currentUID() (uint32, error) {
	return 0, sessionVerificationInconclusive
}

func (systemSessionProcessInspector) list() ([]processRecord, error) {
	return nil, sessionVerificationInconclusive
}

func (systemSessionProcessInspector) inspect(uint32) (*processDetails, error) {
	return nil, sessionVerificationInconclusive
}
