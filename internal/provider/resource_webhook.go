package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ziinc/terraform-provider-eighthundred/internal/provider/client"
)

var (
	_ resource.Resource                = (*webhookResource)(nil)
	_ resource.ResourceWithConfigure   = (*webhookResource)(nil)
	_ resource.ResourceWithImportState = (*webhookResource)(nil)
)

func NewWebhookResource() resource.Resource { return &webhookResource{} }

type webhookResource struct {
	pd *ProviderData
}

type webhookModel struct {
	ID        types.String `tfsdk:"id"`
	CompanyID types.Int64  `tfsdk:"company_id"`
	URL       types.String `tfsdk:"url"`
	Method    types.String `tfsdk:"method"`
	Features  types.List   `tfsdk:"features"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An 800.com webhook subscription. The URL receives an HTTP request whenever a configured feature triggers (currently only 'sms_received').",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Server-assigned webhook ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"company_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Company that owns the webhook. Falls back to the provider's default_company_id.",
			},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "HTTPS URL to receive the webhook payload.",
			},
			"method": schema.StringAttribute{
				Required:    true,
				Description: "HTTP method 800.com uses to deliver the payload. One of GET, POST, PUT, PATCH.",
			},
			"features": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Events that trigger this webhook. Currently only 'sms_received' is supported.",
			},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "internal error: ProviderData type mismatch")
		return
	}
	r.pd = pd
}

func (r *webhookResource) resolveCompanyID(planCompany types.Int64) (int64, error) {
	if !planCompany.IsNull() && !planCompany.IsUnknown() {
		return planCompany.ValueInt64(), nil
	}
	if r.pd.DefaultCompanyID != nil {
		return *r.pd.DefaultCompanyID, nil
	}
	return 0, fmt.Errorf("company_id is required (either set the resource attribute or the provider's default_company_id)")
}

func featuresFromList(ctx context.Context, l types.List) ([]string, error) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	out := make([]string, 0, len(l.Elements()))
	diags := l.ElementsAs(ctx, &out, false)
	if diags.HasError() {
		return nil, fmt.Errorf("decode features: %v", diags)
	}
	return out, nil
}

func featuresToList(ctx context.Context, ss []string) (types.List, error) {
	v, diags := types.ListValueFrom(ctx, types.StringType, ss)
	if diags.HasError() {
		return types.ListNull(types.StringType), fmt.Errorf("encode features: %v", diags)
	}
	return v, nil
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	companyID, err := r.resolveCompanyID(plan.CompanyID)
	if err != nil {
		resp.Diagnostics.AddError("company_id not resolvable", err.Error())
		return
	}
	features, err := featuresFromList(ctx, plan.Features)
	if err != nil {
		resp.Diagnostics.AddError("Invalid features list", err.Error())
		return
	}
	w := client.Webhook{
		URL:      plan.URL.ValueString(),
		Method:   plan.Method.ValueString(),
		Features: features,
	}
	created, err := r.pd.Client.CreateWebhook(ctx, companyID, w)
	if err != nil {
		resp.Diagnostics.AddError("Create webhook failed", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.FormatInt(created.ID, 10))
	plan.CompanyID = types.Int64Value(companyID)
	plan.URL = types.StringValue(created.URL)
	plan.Method = types.StringValue(created.Method)
	if fl, err := featuresToList(ctx, created.Features); err == nil {
		plan.Features = fl
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	companyID, err := r.resolveCompanyID(state.CompanyID)
	if err != nil {
		resp.Diagnostics.AddError("company_id not resolvable", err.Error())
		return
	}
	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid state id", err.Error())
		return
	}
	got, err := r.pd.Client.GetWebhook(ctx, companyID, id)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read webhook failed", err.Error())
		return
	}
	state.URL = types.StringValue(got.URL)
	state.Method = types.StringValue(got.Method)
	if fl, err := featuresToList(ctx, got.Features); err == nil {
		state.Features = fl
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	companyID, err := r.resolveCompanyID(plan.CompanyID)
	if err != nil {
		resp.Diagnostics.AddError("company_id not resolvable", err.Error())
		return
	}
	id, err := strconv.ParseInt(plan.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid state id", err.Error())
		return
	}
	features, err := featuresFromList(ctx, plan.Features)
	if err != nil {
		resp.Diagnostics.AddError("Invalid features list", err.Error())
		return
	}
	updated, err := r.pd.Client.UpdateWebhook(ctx, companyID, id, client.Webhook{
		URL:      plan.URL.ValueString(),
		Method:   plan.Method.ValueString(),
		Features: features,
	})
	if err != nil {
		resp.Diagnostics.AddError("Update webhook failed", err.Error())
		return
	}
	plan.URL = types.StringValue(updated.URL)
	plan.Method = types.StringValue(updated.Method)
	if fl, err := featuresToList(ctx, updated.Features); err == nil {
		plan.Features = fl
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	companyID, err := r.resolveCompanyID(state.CompanyID)
	if err != nil {
		resp.Diagnostics.AddError("company_id not resolvable", err.Error())
		return
	}
	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid state id", err.Error())
		return
	}
	if err := r.pd.Client.DeleteWebhook(ctx, companyID, id); err != nil {
		resp.Diagnostics.AddError("Delete webhook failed", err.Error())
	}
}

// ImportState accepts "<company_id>:<webhook_id>".
func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "expected '<company_id>:<webhook_id>'")
		return
	}
	companyID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid company_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("company_id"), companyID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
