package runtime

import runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"

func doctorQuotaPanel(quota runtimemodel.DoctorQuota) doctorPanel {
	fields := [][2]string{
		{"Current", yesNo(quota.Active)},
		{"Enabled", yesNo(quota.Enabled)},
		{"Provider", valueOrDash(quota.Provider)},
		{"Auth", valueOrDash(quota.Auth)},
	}
	if quota.Error != "" {
		fields = append(fields, [2]string{"Quota", quota.Error})
		return doctorPanel{title: "Profile " + quota.Profile, fields: fields}
	}
	if quota.OpenAI != nil {
		fields = append(fields,
			[2]string{"Quota", quota.OpenAI.HumanStatus},
			[2]string{"Main", quota.OpenAI.Main},
		)
		return doctorPanel{title: "Profile " + quota.Profile, fields: fields}
	}
	if quota.External != nil {
		fields = append(fields,
			[2]string{"Quota", quota.External.Status},
			[2]string{"Main", quota.External.Main},
		)
		if quota.External.Reset != "" {
			fields = append(fields, [2]string{"Reset", quota.External.Reset})
		}
		return doctorPanel{title: "Profile " + quota.Profile, fields: fields}
	}
	fields = append(fields,
		[2]string{"Quota", valueOrDash(quota.State)},
		[2]string{"Plan", valueOrDash(quota.Plan)},
		[2]string{"5h", valueOrDash(quota.FiveHour)},
		[2]string{"Weekly", valueOrDash(quota.Weekly)},
	)
	return doctorPanel{title: "Profile " + quota.Profile, fields: fields}
}
