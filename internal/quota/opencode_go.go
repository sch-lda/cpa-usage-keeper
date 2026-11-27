package quota

import (
	"context"
	"fmt"
	"net/http"

	"cpa-usage-keeper/internal/cpa/dto/response"
)

const openCodeGoProviderID = "opencode-go"

type pluginQuotaReader interface {
	FetchPluginQuota(context.Context, string) (response.PluginQuota, int, error)
}

type openCodeGoProvider struct {
	caller ManagementClient
}

type openCodeGoQuotaRows []QuotaRow

func (p openCodeGoProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	reader, ok := p.caller.(pluginQuotaReader)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("CPA plugin quota reader is unavailable")
	}
	quota, statusCode, err := reader.FetchPluginQuota(ctx, input.Identity.Identity)
	if err != nil {
		if statusCode >= http.StatusBadRequest {
			return ProviderOutput{}, ProviderHTTPError{StatusCode: statusCode}
		}
		return ProviderOutput{}, err
	}

	buckets := make(map[string]response.PluginQuotaBucket)
	for _, group := range quota.Groups {
		for _, bucket := range group.Buckets {
			if fraction := bucket.RemainingFraction; fraction != nil && *fraction >= 0 && *fraction <= 1 {
				buckets[bucket.Window] = bucket
			}
		}
	}
	rows := make(openCodeGoQuotaRows, 0, 3)
	for _, window := range []struct{ name, label string }{
		{"rolling", "5h"}, {"weekly", "Weekly"}, {"monthly", "Monthly"},
	} {
		bucket, ok := buckets[window.name]
		if !ok {
			continue
		}
		rows = append(rows, QuotaRow{
			Key:         "opencode_go." + window.name,
			Label:       window.label,
			UsedPercent: floatPtr((1 - *bucket.RemainingFraction) * 100),
			ResetAt:     bucket.ResetTime,
		})
	}
	if len(rows) == 0 {
		return ProviderOutput{}, fmt.Errorf("OpenCode Go quota returned no usable windows")
	}
	return ProviderOutput{Provider: openCodeGoProviderID, Result: rows}, nil
}
