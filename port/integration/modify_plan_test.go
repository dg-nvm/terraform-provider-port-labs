package integration

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestPlanSpec(t *testing.T) {
	state := types.StringValue(`{"integrationSpec":{"githubToken":"_OLD"},"appSpec":{"liveEventsEnabled":false}}`)

	tests := []struct {
		name   string
		config string
		want   string
	}{
		{
			name:   "inherits appSpec when only integrationSpec changes",
			config: `{"integrationSpec":{"githubToken":"_NEW"}}`,
			want:   `{"appSpec":{"liveEventsEnabled":false},"integrationSpec":{"githubToken":"_NEW"}}`,
		},
		{
			name:   "keeps state when the configuration is unchanged",
			config: `{"integrationSpec":{"githubToken":"_OLD"}}`,
			want:   state.ValueString(),
		},
		{
			name:   "configuration wins when it manages appSpec",
			config: `{"integrationSpec":{"githubToken":"_OLD"},"appSpec":{"liveEventsEnabled":true}}`,
			want:   `{"appSpec":{"liveEventsEnabled":true},"integrationSpec":{"githubToken":"_OLD"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planSpec(types.StringValue(tt.config), state)
			assert.JSONEq(t, tt.want, got.ValueString())
		})
	}
}

func TestExtractAppSpec(t *testing.T) {
	got := extractAppSpec(types.StringValue(
		`{"integrationSpec":{"githubAppPrivateKey":"x"},"appSpec":{"scheduledResyncInterval":"12h"}}`,
	))
	assert.JSONEq(t, `{"appSpec":{"scheduledResyncInterval":"12h"}}`, got.ValueString())

	assert.True(t, extractAppSpec(types.StringValue(`{"integrationSpec":{"token":"x"}}`)).IsNull())
	assert.True(t, extractAppSpec(types.StringNull()).IsNull())
	assert.True(t, extractAppSpec(types.StringUnknown()).IsUnknown())
}

func TestKeepUnconfiguredChangelogDestinations(t *testing.T) {
	webhookType := map[string]attr.Type{"url": types.StringType, "agent": types.BoolType}
	webhook := types.ObjectValueMust(webhookType, map[string]attr.Value{
		"url":   types.StringValue("https://example.com"),
		"agent": types.BoolValue(false),
	})
	otherWebhook := types.ObjectValueMust(webhookType, map[string]attr.Value{
		"url":   types.StringValue("https://other.example.com"),
		"agent": types.BoolValue(false),
	})
	kafkaNull := types.ObjectNull(map[string]attr.Type{})

	t.Run("unset destinations keep state instead of showing as unknown", func(t *testing.T) {
		plan := &IntegrationModel{KafkaChangelogDestination: types.ObjectUnknown(map[string]attr.Type{}), WebhookChangelogDestination: types.ObjectUnknown(webhookType)}
		config := &IntegrationModel{KafkaChangelogDestination: kafkaNull, WebhookChangelogDestination: types.ObjectNull(webhookType)}
		state := &IntegrationModel{KafkaChangelogDestination: kafkaNull, WebhookChangelogDestination: webhook}

		keepUnconfiguredChangelogDestinations(plan, config, state)

		assert.True(t, plan.KafkaChangelogDestination.IsNull())
		assert.True(t, plan.WebhookChangelogDestination.Equal(webhook))
	})

	t.Run("configured destination is planned as configured", func(t *testing.T) {
		plan := &IntegrationModel{KafkaChangelogDestination: kafkaNull, WebhookChangelogDestination: otherWebhook}
		config := &IntegrationModel{KafkaChangelogDestination: kafkaNull, WebhookChangelogDestination: otherWebhook}
		state := &IntegrationModel{KafkaChangelogDestination: kafkaNull, WebhookChangelogDestination: webhook}

		keepUnconfiguredChangelogDestinations(plan, config, state)

		assert.True(t, plan.WebhookChangelogDestination.Equal(otherWebhook))
	})
}
