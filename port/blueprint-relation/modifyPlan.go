package blueprint_relation

import (
	"context"
	"fmt"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/cli"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/consts"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithModifyPlan = &BlueprintRelationResource{}

// ModifyPlan checks, before apply, whether a relation about to be created already exists in Port, so that
// taking it over is never silent: it fails unless adopt_existing is set, and warns when it is.
func (r *BlueprintRelationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.portClient == nil || !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var blueprintIdentifier, relationIdentifier types.String
	var adoptExisting types.Bool
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("blueprint"), &blueprintIdentifier)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("identifier"), &relationIdentifier)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("adopt_existing"), &adoptExisting)...)
	if resp.Diagnostics.HasError() || blueprintIdentifier.IsUnknown() || relationIdentifier.IsUnknown() {
		return
	}

	b, statusCode, err := r.portClient.ReadBlueprint(ctx, blueprintIdentifier.ValueString())
	if err != nil {
		if statusCode != 404 {
			resp.Diagnostics.AddWarning("failed to check whether the relation already exists", err.Error())
		}
		return
	}
	if _, ok := b.Relations[relationIdentifier.ValueString()]; !ok {
		return
	}

	if r.portClient.AdoptExisting(consts.ResourceKindBlueprintRelation, adoptExisting) {
		resp.Diagnostics.AddAttributeWarning(path.Root("adopt_existing"), "relation will be taken over",
			fmt.Sprintf("Relation %q already exists on blueprint %q. Applying will take ownership of it and overwrite it to match the configuration.",
				relationIdentifier.ValueString(), blueprintIdentifier.ValueString()))
		return
	}
	existsErr := &cli.ResourceExistsError{
		ResourceKind: consts.ResourceKindBlueprintRelation,
		Identifier:   relationIdentifier.ValueString(),
		Blueprint:    blueprintIdentifier.ValueString(),
	}
	resp.Diagnostics.AddAttributeError(path.Root("identifier"), existsErr.Summary(), existsErr.Error())
}
