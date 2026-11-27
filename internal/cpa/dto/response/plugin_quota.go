package response

type PluginQuota struct {
	Groups []struct {
		Buckets []PluginQuotaBucket `json:"buckets"`
	} `json:"groups"`
}

type PluginQuotaBucket struct {
	Window            string   `json:"window"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
}
