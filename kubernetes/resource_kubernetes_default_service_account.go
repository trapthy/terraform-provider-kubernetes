package kubernetes

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgApi "k8s.io/apimachinery/pkg/types"
)

func resourceKubernetesDefaultServiceAccount() *schema.Resource {
	serviceAccountResource := resourceKubernetesServiceAccount()

	metaSchema := namespacedMetadataSchema("service account", false)

	nameField := metaSchema.Elem.(*schema.Resource).Schema["name"]
	nameField.Computed = false
	nameField.Default = "default"
	nameField.ValidateFunc = validation.StringInSlice([]string{"default"}, false)

	serviceAccountResource.Schema["metadata"] = metaSchema

	serviceAccountResource.CreateContext = resourceKubernetesDefaultServiceAccountCreate

	return serviceAccountResource
}

func resourceKubernetesDefaultServiceAccountCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn, err := meta.(KubeClientsets).MainClientset()
	if err != nil {
		return diag.FromErr(err)
	}

	metadata := expandMetadata(d.Get("metadata").([]interface{}))
	svcAcc := api.ServiceAccount{ObjectMeta: metadata}

	log.Printf("[INFO] Checking for default service account existence: %s", metadata.Namespace)
	err = resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate), func() *resource.RetryError {
		_, err := conn.CoreV1().ServiceAccounts(metadata.Namespace).Get(ctx, metadata.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("[INFO] Default service account does not exist, will retry: %s", metadata.Namespace)
				return resource.RetryableError(err)
			}

			return resource.NonRetryableError(err)
		}

		log.Printf("[INFO] Default service account exists: %s", metadata.Namespace)
		return nil
	})

	if err != nil {
		return diag.FromErr(err)
	}

	secret, err := getServiceAccountDefaultSecret(ctx, "default", svcAcc, d.Timeout(schema.TimeoutCreate), conn)
	if err != nil {
		return diag.FromErr(err)
	}
	d.Set("default_secret_name", secret.Name)

	ops := patchMetadata("metadata.0.", "/metadata/", d)
	if d.HasChange("image_pull_secret") {
		v := d.Get("image_pull_secret").(*schema.Set).List()
		ops = append(ops, &ReplaceOperation{
			Path:  "/imagePullSecrets",
			Value: expandLocalObjectReferenceArray(v),
		})
	}
	if d.HasChange("secret") {
		v := d.Get("secret").(*schema.Set).List()
		ops = append(ops, &ReplaceOperation{
			Path:  "/secrets",
			Value: expandServiceAccountSecrets(v, secret.Name),
		})
	}

	automountServiceAccountToken := d.Get("automount_service_account_token").(bool)
	ops = append(ops, &ReplaceOperation{
		Path:  "/automountServiceAccountToken",
		Value: automountServiceAccountToken,
	})

	data, err := ops.MarshalJSON()
	if err != nil {
		return diag.Errorf("Failed to marshal update operations: %s", err)
	}
	log.Printf("[INFO] Updating default service account %q: %v", metadata.Name, string(data))
	out, err := conn.CoreV1().ServiceAccounts(metadata.Namespace).Patch(ctx, metadata.Name, pkgApi.JSONPatchType, data, metav1.PatchOptions{})
	if err != nil {
		return diag.Errorf("Failed to update default service account: %s", err)
	}
	log.Printf("[INFO] Submitted updated default service account: %#v", out)

	d.SetId(buildId(metadata))

	return resourceKubernetesServiceAccountRead(ctx, d, meta)
}
