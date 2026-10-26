package page

import (
	"context"
	"fmt"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/cli"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/consts"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithModifyPlan = &PageResource{}

// ModifyPlan checks, before apply, whether a page about to be created already exists in Port, so that
// taking it over is never silent: it fails unless adopt_existing is set, and warns when it is.
func (r *PageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.portClient == nil || !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var identifier, pageType types.String
	var adoptExisting types.Bool
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("identifier"), &identifier)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("type"), &pageType)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("adopt_existing"), &adoptExisting)...)
	if resp.Diagnostics.HasError() || identifier.IsUnknown() || pageType.ValueString() == "entity" {
		return
	}

	_, statusCode, err := r.portClient.GetPage(ctx, identifier.ValueString())
	if err != nil {
		if statusCode != 404 {
			resp.Diagnostics.AddWarning("failed to check whether the page already exists", err.Error())
		}
		return
	}

	if r.portClient.AdoptExisting(consts.ResourceKindPage, adoptExisting) {
		resp.Diagnostics.AddAttributeWarning(path.Root("adopt_existing"), "page will be taken over",
			fmt.Sprintf("Page %q already exists in Port. Applying will take ownership of it and overwrite it to match the configuration.", identifier.ValueString()))
		return
	}
	existsErr := &cli.ResourceExistsError{ResourceKind: consts.ResourceKindPage, Identifier: identifier.ValueString()}
	resp.Diagnostics.AddAttributeError(path.Root("identifier"), existsErr.Summary(), existsErr.Error())
}
