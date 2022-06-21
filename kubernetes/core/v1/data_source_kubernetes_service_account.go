package v1

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/provider"
	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/structures"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DataSourceKubernetesServiceAccount() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceKubernetesServiceAccountRead,

		Schema: map[string]*schema.Schema{
			"metadata": NamespacedMetadataSchema("service account", false),
			"image_pull_secret": {
				Type:        schema.TypeList,
				Description: "A list of references to secrets in the same namespace to use for pulling any images in pods that reference this Service Account. More info: http://kubernetes.io/docs/user-guide/secrets#manually-specifying-an-imagepullsecret",
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Description: "Name of the referent. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
							Computed:    true,
						},
					},
				},
			},
			"secret": {
				Type:        schema.TypeList,
				Description: "A list of secrets allowed to be used by pods running using this Service Account. More info: http://kubernetes.io/docs/user-guide/secrets",
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Description: "Name of the referent. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
							Computed:    true,
						},
					},
				},
			},
			"automount_service_account_token": {
				Type:        schema.TypeBool,
				Description: "True to enable automatic mounting of the service account token",
				Computed:    true,
			},
			"default_secret_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceKubernetesServiceAccountRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn, err := meta.(provider.KubeClientsets).MainClientset()
	if err != nil {
		return diag.FromErr(err)
	}

	metadata := structures.ExpandMetadata(d.Get("metadata").([]interface{}))
	sa, err := conn.CoreV1().ServiceAccounts(metadata.Namespace).Get(ctx, metadata.Name, metav1.GetOptions{})
	if err != nil {
		return diag.Errorf("Unable to fetch service account from Kubernetes: %s", err)
	}

	defaultSecret, diagMsg := findDefaultServiceAccount(ctx, sa, conn)

	err = d.Set("default_secret_name", defaultSecret)
	if err != nil {
		return diag.Errorf("Unable to set default_secret_name: %s", err)
	}

	d.SetId(structures.BuildId(sa.ObjectMeta))

	diagMsg = append(diagMsg, resourceKubernetesServiceAccountRead(ctx, d, meta)...)

	return diagMsg
}
