package v1

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/provider"
	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/structures"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DataSourceKubernetesNamespace() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceKubernetesNamespaceRead,

		Schema: map[string]*schema.Schema{
			"metadata": MetadataSchema("namespace", false),
			"spec": {
				Type:        schema.TypeList,
				Description: "Spec defines the behavior of the Namespace.",
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"finalizers": {
							Type:        schema.TypeList,
							Description: "Finalizers is an opaque list of values that must be empty to permanently remove object from storage.",
							Optional:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
		},
	}
}

func dataSourceKubernetesNamespaceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn, err := meta.(provider.KubeClientsets).MainClientset()
	if err != nil {
		return diag.FromErr(err)
	}

	metadata := structures.ExpandMetadata(d.Get("metadata").([]interface{}))
	d.SetId(metadata.Name)

	namespace, err := conn.CoreV1().Namespaces().Get(ctx, metadata.Name, metav1.GetOptions{})
	if err != nil {
		log.Printf("[DEBUG] Received error: %#v", err)
		return diag.FromErr(err)
	}
	log.Printf("[INFO] Received namespace: %#v", namespace)
	err = d.Set("metadata", structures.FlattenMetadata(namespace.ObjectMeta, d, meta))
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("spec", flattenNamespaceSpec(&namespace.Spec))
	if err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func flattenNamespaceSpec(in *v1.NamespaceSpec) []interface{} {
	if in == nil || len(in.Finalizers) == 0 {
		return []interface{}{}
	}
	spec := make(map[string]interface{})
	fin := make([]string, len(in.Finalizers))
	for i, f := range in.Finalizers {
		fin[i] = string(f)
	}
	spec["finalizers"] = fin
	return []interface{}{spec}
}
