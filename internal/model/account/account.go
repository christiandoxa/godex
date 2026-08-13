package account

type LoginInput struct {
	Name       string
	DeviceAuth bool
}

type DoctorReport struct {
	GodexHome    string
	CodexVersion string
	AccountCount int
	EnabledCount int
}
