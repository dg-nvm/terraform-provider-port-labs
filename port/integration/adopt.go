package integration

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/cli"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/consts"
)

// adoptionConflict reports the first field that cannot be changed after creation and differs between the
// configuration and the existing integration, which makes taking it over impossible
func adoptionConflict(plan *IntegrationModel, existing *cli.Integration) *cli.AdoptionConflictError {
	existingInstallationType := consts.InstallationTypeOnPrem
	if existing.InstallationType != nil && *existing.InstallationType != "" {
		existingInstallationType = *existing.InstallationType
	}
	if plan.installationType() != existingInstallationType {
		return &cli.AdoptionConflictError{
			ResourceKind: consts.ResourceKindIntegration,
			Identifier:   plan.InstallationId.ValueString(),
			Field:        "installation_type",
			Existing:     existingInstallationType,
			Configured:   plan.installationType(),
		}
	}

	existingAppType := ""
	if existing.InstallationAppType != nil {
		existingAppType = *existing.InstallationAppType
	}
	if isKnown(plan.InstallationAppType) && plan.InstallationAppType.ValueString() != existingAppType {
		return &cli.AdoptionConflictError{
			ResourceKind: consts.ResourceKindIntegration,
			Identifier:   plan.InstallationId.ValueString(),
			Field:        "installation_app_type",
			Existing:     existingAppType,
			Configured:   plan.InstallationAppType.ValueString(),
		}
	}
	return nil
}

// checkExistingAtPlan checks, before apply, whether an integration about to be created already exists in Port,
// so that taking it over is never silent: it fails unless adoption is allowed and possible, and warns when it is.
func (r *IntegrationResource) checkExistingAtPlan(ctx context.Context, plan *IntegrationModel, diags *diag.Diagnostics) {
	if r.portClient == nil || !isKnown(plan.InstallationId) {
		return
	}

	existing, statusCode, err := r.portClient.GetIntegration(ctx, plan.InstallationId.ValueString())
	if err != nil {
		if statusCode != 404 {
			diags.AddWarning("failed to check whether the integration already exists", err.Error())
		}
		return
	}

	if !r.portClient.AdoptExisting(consts.ResourceKindIntegration, plan.AdoptExisting) {
		existsErr := &cli.ResourceExistsError{ResourceKind: consts.ResourceKindIntegration, Identifier: plan.InstallationId.ValueString()}
		diags.AddAttributeError(path.Root("installation_id"), existsErr.Summary(), existsErr.Error())
		return
	}
	if conflictErr := adoptionConflict(plan, existing); conflictErr != nil {
		diags.AddAttributeError(path.Root(conflictErr.Field), conflictErr.Summary(), conflictErr.Error())
		return
	}

	diags.AddAttributeWarning(path.Root("adopt_existing"), "integration will be taken over",
		fmt.Sprintf("Integration %q already exists in Port. Applying will take ownership of it and overwrite it to match the configuration.", plan.InstallationId.ValueString()))
	if isKnown(plan.CreatePortResourcesOrigin) {
		diags.AddAttributeWarning(path.Root("create_port_resources_origin"), "create_port_resources_origin is ignored",
			"`create_port_resources_origin` only applies when an integration is created. The existing integration keeps the resources it already created.")
	}
}

// adoptIntegration takes ownership of an integration that already exists in Port by updating it to match the plan
func (r *IntegrationResource) adoptIntegration(ctx context.Context, plan *IntegrationModel, existing *cli.Integration, resp *resource.CreateResponse) {
	installationId := plan.InstallationId.ValueString()
	if !r.portClient.AdoptExisting(consts.ResourceKindIntegration, plan.AdoptExisting) {
		existsErr := &cli.ResourceExistsError{ResourceKind: consts.ResourceKindIntegration, Identifier: installationId}
		resp.Diagnostics.AddError(existsErr.Summary(), existsErr.Error())
		return
	}
	if conflictErr := adoptionConflict(plan, existing); conflictErr != nil {
		resp.Diagnostics.AddError(conflictErr.Summary(), conflictErr.Error())
		return
	}

	tflog.Info(ctx, "integration already exists, adopting it into state", map[string]interface{}{"installation_id": installationId})
	body, err := integrationToPortBody(plan, false)
	if err != nil {
		resp.Diagnostics.AddError("failed to build request body", err.Error())
		return
	}

	updated, err := r.portClient.UpdateIntegration(ctx, installationId, body)
	if err != nil {
		resp.Diagnostics.AddError("failed to take over integration", err.Error())
		return
	}

	applyWriteResult(plan, updated, installationId)

	if plan.isHosted() {
		r.awaitInfra(ctx, plan, installationId, "taken over", true, false, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
