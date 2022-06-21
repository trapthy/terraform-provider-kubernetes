package structures

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/provider"
)

func IdParts(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		err := fmt.Errorf("Unexpected ID format (%q), expected %q.", id, "namespace/name")
		return "", "", err
	}

	return parts[0], parts[1], nil
}

func BuildId(meta metav1.ObjectMeta) string {
	return meta.Namespace + "/" + meta.Name
}

func BuildIdWithVersionKind(meta metav1.ObjectMeta, apiVersion, kind string) string {
	id := fmt.Sprintf("apiVersion=%v,kind=%v,name=%s",
		apiVersion, kind, meta.Name)
	if meta.Namespace != "" {
		id += fmt.Sprintf(",namespace=%v", meta.Namespace)
	}
	return id
}

func ExpandMetadata(in []interface{}) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{}
	if len(in) < 1 {
		return meta
	}
	m := in[0].(map[string]interface{})

	if v, ok := m["annotations"].(map[string]interface{}); ok && len(v) > 0 {
		meta.Annotations = ExpandStringMap(m["annotations"].(map[string]interface{}))
	}

	if v, ok := m["labels"].(map[string]interface{}); ok && len(v) > 0 {
		meta.Labels = ExpandStringMap(m["labels"].(map[string]interface{}))
	}

	if v, ok := m["generate_name"]; ok {
		meta.GenerateName = v.(string)
	}
	if v, ok := m["name"]; ok {
		meta.Name = v.(string)
	}
	if v, ok := m["namespace"]; ok {
		meta.Namespace = v.(string)
	}

	return meta
}

func PatchMetadata(keyPrefix, pathPrefix string, d *schema.ResourceData) PatchOperations {
	ops := make([]PatchOperation, 0, 0)
	if d.HasChange(keyPrefix + "annotations") {
		oldV, newV := d.GetChange(keyPrefix + "annotations")
		diffOps := DiffStringMap(pathPrefix+"annotations", oldV.(map[string]interface{}), newV.(map[string]interface{}))
		ops = append(ops, diffOps...)
	}
	if d.HasChange(keyPrefix + "labels") {
		oldV, newV := d.GetChange(keyPrefix + "labels")
		diffOps := DiffStringMap(pathPrefix+"labels", oldV.(map[string]interface{}), newV.(map[string]interface{}))
		ops = append(ops, diffOps...)
	}
	return ops
}

func ExpandStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v.(string)
	}
	return result
}

func ExpandBase64MapToByteMap(m map[string]interface{}) map[string][]byte {
	result := make(map[string][]byte)
	for k, v := range m {
		b, err := base64.StdEncoding.DecodeString(v.(string))
		if err == nil {
			result[k] = b
		}
	}
	return result
}

func ExpandStringMapToByteMap(m map[string]interface{}) map[string][]byte {
	result := make(map[string][]byte)
	for k, v := range m {
		result[k] = []byte(v.(string))
	}
	return result
}

func ExpandStringSlice(s []interface{}) []string {
	result := make([]string, len(s), len(s))
	for k, v := range s {
		// Handle the Terraform parser bug which turns empty strings in lists to nil.
		if v == nil {
			result[k] = ""
		} else {
			result[k] = v.(string)
		}
	}
	return result
}

func FlattenMetadata(meta metav1.ObjectMeta, d *schema.ResourceData, providerMetadata interface{}, metaPrefix ...string) []interface{} {
	m := make(map[string]interface{})
	prefix := ""
	if len(metaPrefix) > 0 {
		prefix = metaPrefix[0]
	}
	configAnnotations := d.Get(prefix + "metadata.0.annotations").(map[string]interface{})

	var ignoreAnnotations []string
	if v, ok := providerMetadata.(provider.KubeClientsets).ConfigData().Get("ignore_annotations").([]interface{}); ok {
		ignoreAnnotations = ExpandStringSlice(v)
	}

	annotations := removeInternalKeys(meta.Annotations, configAnnotations)
	m["annotations"] = removeKeys(annotations, configAnnotations, ignoreAnnotations)
	if meta.GenerateName != "" {
		m["generate_name"] = meta.GenerateName
	}
	configLabels := d.Get(prefix + "metadata.0.labels").(map[string]interface{})

	var ignoreLabels []string
	if v, ok := providerMetadata.(provider.KubeClientsets).ConfigData().Get("ignore_labels").([]interface{}); ok {
		ignoreLabels = ExpandStringSlice(v)
	}

	labels := removeInternalKeys(meta.Labels, configLabels)
	m["labels"] = removeKeys(labels, configLabels, ignoreLabels)
	m["name"] = meta.Name
	m["resource_version"] = meta.ResourceVersion
	m["uid"] = fmt.Sprintf("%v", meta.UID)
	m["generation"] = meta.Generation

	if meta.Namespace != "" {
		m["namespace"] = meta.Namespace
	}

	return []interface{}{m}
}

func removeInternalKeys(m map[string]string, d map[string]interface{}) map[string]string {
	for k := range m {
		if IsInternalKey(k) && !IsKeyInMap(k, d) {
			delete(m, k)
		}
	}
	return m
}

// removeKeys removes given Kubernetes metadata(annotations and labels) keys.
// In that case, they won't be available in the TF state file and will be ignored during apply/plan operations.
func removeKeys(m map[string]string, d map[string]interface{}, ignoreKubernetesMetadataKeys []string) map[string]string {
	for k := range m {
		if ignoreKey(k, ignoreKubernetesMetadataKeys) && !IsKeyInMap(k, d) {
			delete(m, k)
		}
	}
	return m
}

func IsKeyInMap(key string, d map[string]interface{}) bool {
	if d == nil {
		return false
	}
	for k := range d {
		if k == key {
			return true
		}
	}
	return false
}

func IsInternalKey(annotationKey string) bool {
	u, err := url.Parse("//" + annotationKey)
	if err != nil {
		return false
	}

	// allow user specified application specific keys
	if u.Hostname() == "app.kubernetes.io" {
		return false
	}

	// allow AWS load balancer configuration annotations
	if u.Hostname() == "service.beta.kubernetes.io" {
		return false
	}

	// internal *.kubernetes.io keys
	if strings.HasSuffix(u.Hostname(), "kubernetes.io") {
		return true
	}

	// Specific to DaemonSet annotations, generated & controlled by the server.
	if strings.Contains(annotationKey, "deprecated.daemonset.template.generation") {
		return true
	}
	return false
}

// ignoreKey reports whether the Kubernetes metadata(annotations and labels) key contains
// any match of the regular expression pattern from the expressions slice.
func ignoreKey(key string, expressions []string) bool {
	for _, e := range expressions {
		if ok, _ := regexp.MatchString(e, key); ok {
			return true
		}
	}

	return false
}

func FlattenByteMapToBase64Map(m map[string][]byte) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return result
}

func FlattenByteMapToStringMap(m map[string][]byte) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = string(v)
	}
	return result
}

func PtrToString(s string) *string {
	return &s
}

func PtrToBool(b bool) *bool {
	return &b
}

func PtrToInt32(i int32) *int32 {
	return &i
}

func PtrToInt64(i int64) *int64 {
	return &i
}

func SliceOfString(slice []interface{}) []string {
	result := make([]string, len(slice), len(slice))
	for i, s := range slice {
		result[i] = s.(string)
	}
	return result
}

func Base64EncodeStringMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		value := v.(string)
		result[k] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	return result
}

func Base64EncodeByteMap(m map[string][]byte) map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range m {
		result[k] = base64.StdEncoding.EncodeToString(v)
	}
	return result
}

func Base64DecodeStringMap(m map[string]interface{}) (map[string][]byte, error) {
	mm := map[string][]byte{}
	for k, v := range m {
		d, err := base64.StdEncoding.DecodeString(v.(string))
		if err != nil {
			return nil, err
		}
		mm[k] = []byte(d)
	}
	return mm, nil
}

func FlattenResourceList(l api.ResourceList) map[string]string {
	m := make(map[string]string)
	for k, v := range l {
		m[string(k)] = v.String()
	}
	return m
}

func ExpandMapToResourceList(m map[string]interface{}) (*api.ResourceList, error) {
	out := make(api.ResourceList)
	for stringKey, origValue := range m {
		key := api.ResourceName(stringKey)
		var value resource.Quantity

		if v, ok := origValue.(int); ok {
			q := resource.NewQuantity(int64(v), resource.DecimalExponent)
			value = *q
		} else if v, ok := origValue.(string); ok {
			var err error
			value, err = resource.ParseQuantity(v)
			if err != nil {
				return &out, err
			}
		} else {
			return &out, fmt.Errorf("Unexpected value type: %#v", origValue)
		}

		out[key] = value
	}
	return &out, nil
}

func FlattenPersistentVolumeAccessModes(in []api.PersistentVolumeAccessMode) *schema.Set {
	var out = make([]interface{}, len(in), len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return schema.NewSet(schema.HashString, out)
}

func ExpandPersistentVolumeAccessModes(s []interface{}) []api.PersistentVolumeAccessMode {
	out := make([]api.PersistentVolumeAccessMode, len(s), len(s))
	for i, v := range s {
		out[i] = api.PersistentVolumeAccessMode(v.(string))
	}
	return out
}

func FlattenResourceQuotaSpec(in api.ResourceQuotaSpec) []interface{} {
	out := make([]interface{}, 1)

	m := make(map[string]interface{}, 0)
	m["hard"] = FlattenResourceList(in.Hard)
	m["scopes"] = FlattenResourceQuotaScopes(in.Scopes)

	if in.ScopeSelector != nil {
		m["scope_selector"] = FlattenResourceQuotaScopeSelector(in.ScopeSelector)
	}

	out[0] = m
	return out
}

func ExpandResourceQuotaSpec(s []interface{}) (*api.ResourceQuotaSpec, error) {
	out := &api.ResourceQuotaSpec{}
	if len(s) < 1 {
		return out, nil
	}
	m := s[0].(map[string]interface{})

	if v, ok := m["hard"]; ok {
		list, err := ExpandMapToResourceList(v.(map[string]interface{}))
		if err != nil {
			return out, err
		}
		out.Hard = *list
	}

	if v, ok := m["scopes"]; ok {
		out.Scopes = ExpandResourceQuotaScopes(v.(*schema.Set).List())
	}

	if v, ok := m["scope_selector"]; ok {
		out.ScopeSelector = ExpandResourceQuotaScopeSelector(v.([]interface{}))
	}

	return out, nil
}

func FlattenResourceQuotaScopes(in []api.ResourceQuotaScope) *schema.Set {
	out := make([]string, len(in), len(in))
	for i, scope := range in {
		out[i] = string(scope)
	}
	return NewStringSet(schema.HashString, out)
}

func ExpandResourceQuotaScopes(s []interface{}) []api.ResourceQuotaScope {
	out := make([]api.ResourceQuotaScope, len(s), len(s))
	for i, scope := range s {
		out[i] = api.ResourceQuotaScope(scope.(string))
	}
	return out
}

func ExpandResourceQuotaScopeSelector(s []interface{}) *api.ScopeSelector {
	if len(s) < 1 {
		return nil
	}
	m := s[0].(map[string]interface{})

	att := &api.ScopeSelector{}

	if v, ok := m["match_expression"].([]interface{}); ok {
		att.MatchExpressions = ExpandResourceQuotaScopeSelectorMatchExpressions(v)
	}

	return att
}

func ExpandResourceQuotaScopeSelectorMatchExpressions(s []interface{}) []api.ScopedResourceSelectorRequirement {
	out := make([]api.ScopedResourceSelectorRequirement, len(s), len(s))

	for i, raw := range s {
		matchExp := raw.(map[string]interface{})

		if v, ok := matchExp["scope_name"].(string); ok {
			out[i].ScopeName = api.ResourceQuotaScope(v)
		}

		if v, ok := matchExp["operator"].(string); ok {
			out[i].Operator = api.ScopeSelectorOperator(v)
		}

		if v, ok := matchExp["values"].(*schema.Set); ok && v.Len() > 0 {
			out[i].Values = SliceOfString(v.List())
		}
	}
	return out
}

func FlattenResourceQuotaScopeSelector(in *api.ScopeSelector) []interface{} {
	out := make([]interface{}, 1)

	m := make(map[string]interface{}, 0)
	m["match_expression"] = FlattenResourceQuotaScopeSelectorMatchExpressions(in.MatchExpressions)

	out[0] = m
	return out
}

func FlattenResourceQuotaScopeSelectorMatchExpressions(in []api.ScopedResourceSelectorRequirement) []interface{} {
	if len(in) == 0 {
		return []interface{}{}
	}
	out := make([]interface{}, len(in))

	for i, l := range in {
		m := make(map[string]interface{}, 0)
		m["operator"] = string(l.Operator)
		m["scope_name"] = string(l.ScopeName)

		if l.Values != nil && len(l.Values) > 0 {
			m["values"] = NewStringSet(schema.HashString, l.Values)
		}

		out[i] = m
	}
	return out
}

func NewStringSet(f schema.SchemaSetFunc, in []string) *schema.Set {
	var out = make([]interface{}, len(in), len(in))
	for i, v := range in {
		out[i] = v
	}
	return schema.NewSet(f, out)
}
func NewInt64Set(f schema.SchemaSetFunc, in []int64) *schema.Set {
	var out = make([]interface{}, len(in), len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return schema.NewSet(f, out)
}

func ResourceListEquals(x, y api.ResourceList) bool {
	for k, v := range x {
		yValue, ok := y[k]
		if !ok {
			return false
		}
		if v.Cmp(yValue) != 0 {
			return false
		}
	}
	for k, v := range y {
		xValue, ok := x[k]
		if !ok {
			return false
		}
		if v.Cmp(xValue) != 0 {
			return false
		}
	}
	return true
}

func ExpandLimitRangeSpec(s []interface{}, isNew bool) (*api.LimitRangeSpec, error) {
	out := &api.LimitRangeSpec{}
	if len(s) < 1 || s[0] == nil {
		return out, nil
	}
	m := s[0].(map[string]interface{})

	if limits, ok := m["limit"].([]interface{}); ok {
		newLimits := make([]api.LimitRangeItem, len(limits), len(limits))

		for i, l := range limits {
			lrItem := api.LimitRangeItem{}
			limit := l.(map[string]interface{})

			if v, ok := limit["type"]; ok {
				lrItem.Type = api.LimitType(v.(string))
			}

			// defaultRequest is forbidden for Pod limits, even though it's set & returned by API
			// this is how we avoid sending it back
			if v, ok := limit["default_request"]; ok {
				drm := v.(map[string]interface{})
				if lrItem.Type == api.LimitTypePod && len(drm) > 0 {
					if isNew {
						return out, fmt.Errorf("limit.%d.default_request cannot be set for Pod limit", i)
					}
				} else {
					el, err := ExpandMapToResourceList(drm)
					if err != nil {
						return out, err
					}
					lrItem.DefaultRequest = *el
				}
			}

			if v, ok := limit["default"]; ok {
				el, err := ExpandMapToResourceList(v.(map[string]interface{}))
				if err != nil {
					return out, err
				}
				lrItem.Default = *el
			}
			if v, ok := limit["max"]; ok {
				el, err := ExpandMapToResourceList(v.(map[string]interface{}))
				if err != nil {
					return out, err
				}
				lrItem.Max = *el
			}
			if v, ok := limit["max_limit_request_ratio"]; ok {
				el, err := ExpandMapToResourceList(v.(map[string]interface{}))
				if err != nil {
					return out, err
				}
				lrItem.MaxLimitRequestRatio = *el
			}
			if v, ok := limit["min"]; ok {
				el, err := ExpandMapToResourceList(v.(map[string]interface{}))
				if err != nil {
					return out, err
				}
				lrItem.Min = *el
			}

			newLimits[i] = lrItem
		}

		out.Limits = newLimits
	}

	return out, nil
}

func FlattenLimitRangeSpec(in api.LimitRangeSpec) []interface{} {
	if len(in.Limits) == 0 {
		return []interface{}{}
	}

	out := make([]interface{}, 1)
	limits := make([]interface{}, len(in.Limits), len(in.Limits))

	for i, l := range in.Limits {
		m := make(map[string]interface{}, 0)
		m["default"] = FlattenResourceList(l.Default)
		m["default_request"] = FlattenResourceList(l.DefaultRequest)
		m["max"] = FlattenResourceList(l.Max)
		m["max_limit_request_ratio"] = FlattenResourceList(l.MaxLimitRequestRatio)
		m["min"] = FlattenResourceList(l.Min)
		m["type"] = string(l.Type)

		limits[i] = m
	}
	out[0] = map[string]interface{}{
		"limit": limits,
	}
	return out
}

func SchemaSetToStringArray(set *schema.Set) []string {
	array := make([]string, 0, set.Len())
	for _, elem := range set.List() {
		e := elem.(string)
		array = append(array, e)
	}
	return array
}

func SchemaSetToInt64Array(set *schema.Set) []int64 {
	array := make([]int64, 0, set.Len())
	for _, elem := range set.List() {
		e := elem.(int)
		array = append(array, int64(e))
	}
	return array
}
func FlattenLabelSelectorRequirementList(l []metav1.LabelSelectorRequirement) []interface{} {
	att := make([]map[string]interface{}, len(l))
	for i, v := range l {
		m := map[string]interface{}{}
		m["key"] = v.Key
		m["values"] = NewStringSet(schema.HashString, v.Values)
		m["operator"] = string(v.Operator)
		att[i] = m
	}
	return []interface{}{att}
}

func FlattenLocalObjectReferenceArray(in []api.LocalObjectReference) []interface{} {
	att := make([]interface{}, len(in))
	for i, v := range in {
		m := map[string]interface{}{}
		if v.Name != "" {
			m["name"] = v.Name
		}
		att[i] = m
	}
	return att
}

func ExpandLocalObjectReferenceArray(in []interface{}) []api.LocalObjectReference {
	att := []api.LocalObjectReference{}
	if len(in) < 1 {
		return att
	}
	att = make([]api.LocalObjectReference, len(in))
	for i, c := range in {
		p := c.(map[string]interface{})
		if name, ok := p["name"]; ok {
			att[i].Name = name.(string)
		}
	}
	return att
}

func FlattenServiceAccountSecrets(in []api.ObjectReference, defaultSecretName string) []interface{} {
	att := make([]interface{}, 0)
	for _, v := range in {
		if v.Name == defaultSecretName {
			continue
		}
		m := map[string]interface{}{}
		if v.Name != "" {
			m["name"] = v.Name
		}
		att = append(att, m)
	}
	return att
}

func ExpandServiceAccountSecrets(in []interface{}, defaultSecretName string) []api.ObjectReference {
	att := make([]api.ObjectReference, 0)

	for _, c := range in {
		p := c.(map[string]interface{})
		if name, ok := p["name"]; ok {
			att = append(att, api.ObjectReference{Name: name.(string)})
		}
	}
	if defaultSecretName != "" {
		att = append(att, api.ObjectReference{Name: defaultSecretName})
	}

	return att
}

func FlattenNodeSelectorRequirementList(in []api.NodeSelectorRequirement) []map[string]interface{} {
	att := make([]map[string]interface{}, len(in))
	for i, v := range in {
		m := map[string]interface{}{}
		m["key"] = v.Key
		m["values"] = NewStringSet(schema.HashString, v.Values)
		m["operator"] = string(v.Operator)
		att[i] = m
	}
	return att
}

func ExpandNodeSelectorRequirementList(in []interface{}) []api.NodeSelectorRequirement {
	att := []api.NodeSelectorRequirement{}
	if len(in) < 1 {
		return att
	}
	att = make([]api.NodeSelectorRequirement, len(in))
	for i, c := range in {
		p := c.(map[string]interface{})
		att[i].Key = p["key"].(string)
		att[i].Operator = api.NodeSelectorOperator(p["operator"].(string))
		att[i].Values = ExpandStringSlice(p["values"].(*schema.Set).List())
	}
	return att
}

func FlattenNodeSelectorTerm(in api.NodeSelectorTerm) []interface{} {
	att := make(map[string]interface{})
	if len(in.MatchExpressions) > 0 {
		att["match_expressions"] = FlattenNodeSelectorRequirementList(in.MatchExpressions)
	}
	if len(in.MatchFields) > 0 {
		att["match_fields"] = FlattenNodeSelectorRequirementList(in.MatchFields)
	}
	return []interface{}{att}
}

func ExpandNodeSelectorTerm(l []interface{}) *api.NodeSelectorTerm {
	if len(l) == 0 || l[0] == nil {
		return &api.NodeSelectorTerm{}
	}
	in := l[0].(map[string]interface{})
	obj := api.NodeSelectorTerm{}
	if v, ok := in["match_expressions"].([]interface{}); ok && len(v) > 0 {
		obj.MatchExpressions = ExpandNodeSelectorRequirementList(v)
	}
	if v, ok := in["match_fields"].([]interface{}); ok && len(v) > 0 {
		obj.MatchFields = ExpandNodeSelectorRequirementList(v)
	}
	return &obj
}

func FlattenNodeSelectorTerms(in []api.NodeSelectorTerm) []interface{} {
	att := make([]interface{}, len(in), len(in))
	for i, n := range in {
		att[i] = FlattenNodeSelectorTerm(n)[0]
	}
	return att
}

func ExpandNodeSelectorTerms(l []interface{}) []api.NodeSelectorTerm {
	if len(l) == 0 || l[0] == nil {
		return []api.NodeSelectorTerm{}
	}
	obj := make([]api.NodeSelectorTerm, len(l), len(l))
	for i, n := range l {
		obj[i] = *ExpandNodeSelectorTerm([]interface{}{n})
	}
	return obj
}

func FlattenPersistentVolumeMountOptions(in []string) *schema.Set {
	var out = make([]interface{}, len(in), len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return schema.NewSet(schema.HashString, out)
}
