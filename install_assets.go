package godexassets

import _ "embed"

// InstallSH is the release installer embedded into Godex for self-update.
//
//go:embed install.sh
var InstallSH []byte

// InstallPowerShell is the Windows release installer embedded into Godex for self-update.
//
//go:embed install.ps1
var InstallPowerShell []byte
