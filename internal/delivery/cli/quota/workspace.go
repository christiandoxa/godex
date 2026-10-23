package quota

import (
	"strings"
	"unicode"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func quotaWorkspaceLabel(workspace *quotamodel.Workspace) string {
	if workspace == nil {
		return ""
	}
	if name := workspaceDisplayText(workspace.Name); name != "" {
		return name
	}
	id := workspaceDisplayText(workspace.ID)
	runes := []rune(id)
	if len(runes) > 24 {
		return string(runes[:12]) + "..." + string(runes[len(runes)-6:])
	}
	return id
}

func workspaceDisplayText(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value))
}
