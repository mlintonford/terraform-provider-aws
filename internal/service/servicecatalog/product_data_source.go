// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

// DONOTCOPY: Copying old resources spreads bad habits. Use skaff instead.

package servicecatalog

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	awstypes "github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKDataSource("aws_servicecatalog_product", name="Product")
// @Tags
// @Testing(tagsIdentifierAttribute="id")
// @Testing(tagsIdentifierAttribute="id", tagsResourceType="Product")
func dataSourceProduct() *schema.Resource {
	return &schema.Resource{
		ReadWithoutTimeout: dataSourceProductRead,

		Timeouts: &schema.ResourceTimeout{
			Read: schema.DefaultTimeout(ProductReadTimeout),
		},

		Schema: map[string]*schema.Schema{
			names.AttrARN: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"accept_language": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      acceptLanguageEnglish,
				ValidateFunc: validation.StringInSlice(acceptLanguage_Values(), false),
			},
			names.AttrCreatedTime: {
				Type:     schema.TypeString,
				Computed: true,
			},
			names.AttrDescription: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"distributor": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"has_default_path": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			names.AttrID: {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{names.AttrID, names.AttrName},
			},
			names.AttrName: {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{names.AttrID, names.AttrName},
				RequiredWith: []string{"portfolio_name"},
			},
			names.AttrOwner: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"portfolio_name": {
				Type:          schema.TypeString,
				Optional:      true,
				RequiredWith:  []string{names.AttrName},
				ConflictsWith: []string{names.AttrID},
			},
			names.AttrStatus: {
				Type:     schema.TypeString,
				Computed: true,
			},
			"support_description": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"support_email": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"support_url": {
				Type:     schema.TypeString,
				Computed: true,
			},
			names.AttrTags: tftags.TagsSchemaComputed(),
			names.AttrType: {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceProductRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).ServiceCatalogClient(ctx)
	acceptLanguage := d.Get("accept_language").(string)
	productID := d.Get(names.AttrID).(string)

	if productID == "" {
		portfolio, err := findPortfolioByName(ctx, conn, acceptLanguage, d.Get("portfolio_name").(string))
		if err != nil {
			return sdkdiag.AppendFromErr(diags, tfresource.SingularDataSourceFindError("Service Catalog Portfolio", err))
		}

		product, err := findProductViewSummaryByNameAndPortfolioID(ctx, conn, acceptLanguage, d.Get(names.AttrName).(string), aws.ToString(portfolio.Id))
		if err != nil {
			return sdkdiag.AppendFromErr(diags, tfresource.SingularDataSourceFindError("Service Catalog Product", err))
		}

		productID = aws.ToString(product.ProductId)
	}

	output, err := waitProductReady(ctx, conn, acceptLanguage, productID, d.Timeout(schema.TimeoutRead))

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "describing Service Catalog Product: %s", err)
	}

	if output == nil || output.ProductViewDetail == nil || output.ProductViewDetail.ProductViewSummary == nil {
		return sdkdiag.AppendErrorf(diags, "getting Service Catalog Product: empty response")
	}

	pvs := output.ProductViewDetail.ProductViewSummary

	d.Set(names.AttrARN, output.ProductViewDetail.ProductARN)
	if output.ProductViewDetail.CreatedTime != nil {
		d.Set(names.AttrCreatedTime, output.ProductViewDetail.CreatedTime.Format(time.RFC3339))
	}
	d.Set(names.AttrDescription, pvs.ShortDescription)
	d.Set("distributor", pvs.Distributor)
	d.Set("has_default_path", pvs.HasDefaultPath)
	d.Set(names.AttrName, pvs.Name)
	d.Set(names.AttrOwner, pvs.Owner)
	d.Set(names.AttrStatus, output.ProductViewDetail.Status)
	d.Set("support_description", pvs.SupportDescription)
	d.Set("support_email", pvs.SupportEmail)
	d.Set("support_url", pvs.SupportUrl)
	d.Set(names.AttrType, pvs.Type)

	d.SetId(aws.ToString(pvs.ProductId))

	setTagsOut(ctx, output.Tags)

	return diags
}

func findPortfolioByName(ctx context.Context, conn *servicecatalog.Client, acceptLanguage, name string) (*awstypes.PortfolioDetail, error) {
	input := &servicecatalog.ListPortfoliosInput{
		AcceptLanguage: aws.String(acceptLanguage),
	}
	paginator := servicecatalog.NewListPortfoliosPaginator(conn, input)
	var output []awstypes.PortfolioDetail

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, portfolio := range page.PortfolioDetails {
			if aws.ToString(portfolio.DisplayName) == name {
				output = append(output, portfolio)
			}
		}
	}

	return tfresource.AssertSingleValueResult(output)
}

func findProductViewSummaryByNameAndPortfolioID(ctx context.Context, conn *servicecatalog.Client, acceptLanguage, name, portfolioID string) (*awstypes.ProductViewSummary, error) {
	input := &servicecatalog.SearchProductsAsAdminInput{
		AcceptLanguage: aws.String(acceptLanguage),
		PortfolioId:    aws.String(portfolioID),
	}
	paginator := servicecatalog.NewSearchProductsAsAdminPaginator(conn, input)
	var output []awstypes.ProductViewSummary

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, detail := range page.ProductViewDetails {
			if detail.ProductViewSummary != nil && aws.ToString(detail.ProductViewSummary.Name) == name {
				output = append(output, *detail.ProductViewSummary)
			}
		}
	}

	return tfresource.AssertSingleValueResult(output)
}
