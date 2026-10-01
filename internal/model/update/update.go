package update

type Decision string

const (
	UpToDate        Decision = "up_to_date"
	UpdateAvailable Decision = "update_available"
	LocalNewer      Decision = "local_newer"
	Updated         Decision = "updated"
)

type Report struct {
	Installed string
	Latest    string
	Status    Decision
	Stdout    string
	Stderr    string
}

type InstallResult struct {
	Stdout string
	Stderr string
}
