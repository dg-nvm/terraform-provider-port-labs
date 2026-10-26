package page

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/adopttest"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/cli"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/consts"
	"github.com/stretchr/testify/assert"
)

func pagePlanValues(adoptExisting *bool) map[string]tftypes.Value {
	values := map[string]tftypes.Value{
		"identifier": adopttest.String("services"),
		"type":       adopttest.String("blueprint-entities"),
		"blueprint":  adopttest.String("service"),
		"title":      adopttest.String("Services"),
	}
	if adoptExisting != nil {
		values["adopt_existing"] = adopttest.Bool(*adoptExisting)
	}
	return values
}

func ptr(b bool) *bool { return &b }

var adoptionCases = []struct {
	name          string
	exists        bool
	policy        cli.AdoptionPolicy
	adoptExisting *bool
	wantError     string
	wantWarning   string
}{
	{name: "missing page is created", exists: false},
	{name: "missing page with adoption allowed is created", exists: false, adoptExisting: ptr(true)},
	{name: "existing page without opt-in fails", exists: true, wantError: "page already exists"},
	{name: "existing page with resource opt-in is taken over", exists: true, adoptExisting: ptr(true), wantWarning: "page will be taken over"},
	{name: "existing page with provider policy is taken over", exists: true, policy: adopttest.Policy(consts.ResourceKindPage), wantWarning: "page will be taken over"},
	{name: "provider policy for another kind does not apply", exists: true, policy: adopttest.Policy(consts.ResourceKindBlueprint), wantError: "page already exists"},
	{name: "resource opt-out overrides provider policy", exists: true, policy: adopttest.Policy(consts.ResourceKindPage), adoptExisting: ptr(false), wantError: "page already exists"},
}

func TestPageModifyPlanAdoption(t *testing.T) {
	for _, tc := range adoptionCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := adopttest.NewFakePort(t)
			if tc.exists {
				fake.AddPage("services", map[string]any{"identifier": "services", "type": "blueprint-entities"})
			}
			r := &PageResource{portClient: fake.Client(tc.policy)}
			s := adopttest.ResourceSchema(t, r)
			plan := adopttest.Plan(t, s, pagePlanValues(tc.adoptExisting))

			resp := &resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: adopttest.NullState(s)}, resp)

			assertDiagnostics(t, tc.wantError, tc.wantWarning, adopttest.ErrorSummaries(resp.Diagnostics), adopttest.WarningSummaries(resp.Diagnostics))
			assert.Empty(t, fake.Writes(), "planning must never write to Port")
		})
	}
}

func TestPageCreateAdoption(t *testing.T) {
	for _, tc := range adoptionCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := adopttest.NewFakePort(t)
			if tc.exists {
				fake.AddPage("services", map[string]any{"identifier": "services", "type": "blueprint-entities"})
			}
			r := &PageResource{portClient: fake.Client(tc.policy)}
			s := adopttest.ResourceSchema(t, r)
			plan := adopttest.Plan(t, s, pagePlanValues(tc.adoptExisting))

			resp := &resource.CreateResponse{State: adopttest.NullState(s)}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, resp)

			switch {
			case tc.wantError != "":
				assert.Equal(t, []string{tc.wantError}, adopttest.ErrorSummaries(resp.Diagnostics))
				assert.Empty(t, fake.Writes(), "a page that may not be taken over must not be written")
				assert.True(t, resp.State.Raw.IsNull())
			case tc.exists:
				assert.Empty(t, adopttest.ErrorSummaries(resp.Diagnostics))
				assert.Equal(t, []string{"PUT /v1/pages/services"}, fake.Writes())
			default:
				assert.Empty(t, adopttest.ErrorSummaries(resp.Diagnostics))
				assert.Equal(t, []string{"POST /v1/pages"}, fake.Writes())
			}
		})
	}
}

func TestPageModifyPlanSkipsEntityPages(t *testing.T) {
	fake := adopttest.NewFakePort(t)
	fake.AddPage("service_entity", map[string]any{"identifier": "service_entity", "type": "entity"})
	r := &PageResource{portClient: fake.Client(nil)}
	s := adopttest.ResourceSchema(t, r)
	plan := adopttest.Plan(t, s, map[string]tftypes.Value{
		"identifier": adopttest.String("service_entity"),
		"type":       adopttest.String("entity"),
	})

	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: adopttest.NullState(s)}, resp)

	assert.Empty(t, resp.Diagnostics, "entity pages always exist with their blueprint and are always taken over")
}

func TestPageModifyPlanSkipsUpdates(t *testing.T) {
	fake := adopttest.NewFakePort(t)
	fake.AddPage("services", map[string]any{"identifier": "services"})
	r := &PageResource{portClient: fake.Client(nil)}
	s := adopttest.ResourceSchema(t, r)
	plan := adopttest.Plan(t, s, pagePlanValues(nil))

	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: adopttest.State(plan)}, resp)

	assert.Empty(t, resp.Diagnostics, "a page already in state is not checked again")
}

func assertDiagnostics(t *testing.T, wantError, wantWarning string, errors, warnings []string) {
	t.Helper()
	if wantError == "" {
		assert.Empty(t, errors)
	} else {
		assert.Equal(t, []string{wantError}, errors)
	}
	if wantWarning == "" {
		assert.Empty(t, warnings)
	} else {
		assert.Equal(t, []string{wantWarning}, warnings)
	}
}

func TestPageCreateAdoptsCatalogPageCreatedConcurrently(t *testing.T) {
	testCases := []struct {
		name       string
		policy     cli.AdoptionPolicy
		wantError  string
		wantWrites []string
	}{
		{name: "without opt-in fails", wantError: "page already exists", wantWrites: []string{"POST /v1/pages"}},
		{name: "with provider policy is taken over", policy: adopttest.Policy(consts.ResourceKindPage), wantWrites: []string{"POST /v1/pages", "PUT /v1/pages/services"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := adopttest.NewFakePort(t)
			fake.PageAppearsOnCreate("services", map[string]any{"identifier": "services", "type": "blueprint-entities"})
			r := &PageResource{portClient: fake.Client(tc.policy)}
			s := adopttest.ResourceSchema(t, r)
			plan := adopttest.Plan(t, s, pagePlanValues(nil))

			resp := &resource.CreateResponse{State: adopttest.NullState(s)}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, resp)

			if tc.wantError == "" {
				assert.Empty(t, adopttest.ErrorSummaries(resp.Diagnostics))
				assert.False(t, resp.State.Raw.IsNull())
			} else {
				assert.Equal(t, []string{tc.wantError}, adopttest.ErrorSummaries(resp.Diagnostics))
				assert.True(t, resp.State.Raw.IsNull())
			}
			assert.Equal(t, tc.wantWrites, fake.Writes())
		})
	}
}

func TestRefreshPageWidgets(t *testing.T) {
	widget := map[string]any{"id": "table", "type": "table-entities-explorer"}
	testCases := []struct {
		name        string
		stateNull   bool
		apiWidgets  *[]map[string]any
		wantNull    bool
		wantEntries int
	}{
		{name: "no widgets declared and none in Port stays undeclared", stateNull: true, apiWidgets: &[]map[string]any{}, wantNull: true},
		{name: "no widgets declared but some in Port shows them", stateNull: true, apiWidgets: &[]map[string]any{widget}, wantEntries: 1},
		{name: "declared widgets removed in Port shows the removal", stateNull: false, apiWidgets: &[]map[string]any{}, wantEntries: 0},
		{name: "missing widgets field is null", stateNull: false, apiWidgets: nil, wantNull: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &PageResource{portClient: &cli.PortClient{}}
			state := &PageModel{Widgets: types.ListNull(types.StringType)}
			if !tc.stateNull {
				state.Widgets = types.ListValueMust(types.StringType, []attr.Value{types.StringValue(`{"id":"table"}`)})
			}

			err := r.refreshPageToState(state, &cli.Page{Identifier: "services", Widgets: tc.apiWidgets})

			assert.NoError(t, err)
			assert.Equal(t, tc.wantNull, state.Widgets.IsNull())
			if !tc.wantNull {
				assert.Len(t, state.Widgets.Elements(), tc.wantEntries)
			}
		})
	}
}
