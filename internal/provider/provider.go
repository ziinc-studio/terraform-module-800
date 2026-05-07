package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ziinc/terraform-provider-eighthundred/internal/provider/client"
)

var _ provider.Provider = (*eightHundredProvider)(nil)

type eightHundredProvider struct {
	version string
}

type providerModel struct {
	Endpoint              types.String `tfsdk:"endpoint"`
	Token                 types.String `tfsdk:"token"`
	DefaultCompanyID      types.Int64  `tfsdk:"default_company_id"`
	MaxRetries            types.Int64  `tfsdk:"max_retries"`
	RequestTimeoutSeconds types.Int64  `tfsdk:"request_timeout_seconds"`
}

// ProviderData is what every resource and data source receives in
// Configure. The default_company_id is plumbed through here so resources
// can fall back to it when their own company_id is unset.
type ProviderData struct {
	Client             *client.Client
	DefaultCompanyID   *int64
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &eightHundredProvider{version: version}
	}
}

func (p *eightHundredProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "eighthundred"
	resp.Version = p.version
}

func (p *eightHundredProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for the 800.com public REST API. Authenticate with a Personal Access Token from your 800.com User Settings.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the 800.com API. Defaults to https://api.800.com. Override for staging or development environments.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Personal Access Token. May also be supplied via the EIGHT_HUNDRED_API_TOKEN environment variable.",
			},
			"default_company_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Default company ID for company-scoped resources. May also be supplied via EIGHT_HUNDRED_COMPANY_ID.",
			},
			"max_retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum retries on 429/5xx responses. Defaults to 3.",
			},
			"request_timeout_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Per-request HTTP timeout, in seconds. Defaults to 30.",
			},
		},
	}
}

func (p *eightHundredProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := "https://api.800.com"
	if !cfg.Endpoint.IsNull() && cfg.Endpoint.ValueString() != "" {
		endpoint = cfg.Endpoint.ValueString()
	}

	token := os.Getenv("EIGHT_HUNDRED_API_TOKEN")
	if !cfg.Token.IsNull() && cfg.Token.ValueString() != "" {
		token = cfg.Token.ValueString()
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing 800.com API token",
			"Set the provider 'token' attribute or the EIGHT_HUNDRED_API_TOKEN environment variable. "+
				"Tokens are minted under User Settings in the 800.com web app.",
		)
		return
	}

	maxRetries := int64(3)
	if !cfg.MaxRetries.IsNull() {
		maxRetries = cfg.MaxRetries.ValueInt64()
	}
	timeout := int64(30)
	if !cfg.RequestTimeoutSeconds.IsNull() {
		timeout = cfg.RequestTimeoutSeconds.ValueInt64()
	}

	c, err := client.New(client.Config{
		Endpoint:              endpoint,
		Token:                 token,
		MaxRetries:            int(maxRetries),
		RequestTimeoutSeconds: int(timeout),
		UserAgent:             "terraform-provider-eighthundred/" + p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to construct 800.com client", err.Error())
		return
	}

	pd := &ProviderData{Client: c}
	if !cfg.DefaultCompanyID.IsNull() {
		v := cfg.DefaultCompanyID.ValueInt64()
		pd.DefaultCompanyID = &v
	} else if env := os.Getenv("EIGHT_HUNDRED_COMPANY_ID"); env != "" {
		// best-effort parse; bad value is the user's problem and surfaces on first call
		var v int64
		_, err := fmtScan(env, &v)
		if err == nil {
			pd.DefaultCompanyID = &v
		}
	}

	resp.DataSourceData = pd
	resp.ResourceData = pd
}

func (p *eightHundredProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWebhookResource,
	}
}

func (p *eightHundredProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
