package v1

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-kubernetes/kubernetes/structures"

	corev1 "k8s.io/api/core/v1"
)

func flattenNodeSelectorRequirementList(in []corev1.NodeSelectorRequirement) []map[string]interface{} {
	att := make([]map[string]interface{}, len(in))
	for i, v := range in {
		m := map[string]interface{}{}
		m["key"] = v.Key
		m["values"] = structures.NewStringSet(schema.HashString, v.Values)
		m["operator"] = string(v.Operator)
		att[i] = m
	}
	return att
}

func expandNodeSelectorRequirementList(in []interface{}) []corev1.NodeSelectorRequirement {
	att := []corev1.NodeSelectorRequirement{}
	if len(in) < 1 {
		return att
	}
	att = make([]corev1.NodeSelectorRequirement, len(in))
	for i, c := range in {
		p := c.(map[string]interface{})
		att[i].Key = p["key"].(string)
		att[i].Operator = corev1.NodeSelectorOperator(p["operator"].(string))
		att[i].Values = structures.ExpandStringSlice(p["values"].(*schema.Set).List())
	}
	return att
}

func flattenNodeSelectorTerm(in corev1.NodeSelectorTerm) []interface{} {
	att := make(map[string]interface{})
	if len(in.MatchExpressions) > 0 {
		att["match_expressions"] = flattenNodeSelectorRequirementList(in.MatchExpressions)
	}
	if len(in.MatchFields) > 0 {
		att["match_fields"] = flattenNodeSelectorRequirementList(in.MatchFields)
	}
	return []interface{}{att}
}

func expandNodeSelectorTerm(l []interface{}) *corev1.NodeSelectorTerm {
	if len(l) == 0 || l[0] == nil {
		return &corev1.NodeSelectorTerm{}
	}
	in := l[0].(map[string]interface{})
	obj := corev1.NodeSelectorTerm{}
	if v, ok := in["match_expressions"].([]interface{}); ok && len(v) > 0 {
		obj.MatchExpressions = expandNodeSelectorRequirementList(v)
	}
	if v, ok := in["match_fields"].([]interface{}); ok && len(v) > 0 {
		obj.MatchFields = expandNodeSelectorRequirementList(v)
	}
	return &obj
}

func flattenNodeSelectorTerms(in []corev1.NodeSelectorTerm) []interface{} {
	att := make([]interface{}, len(in), len(in))
	for i, n := range in {
		att[i] = flattenNodeSelectorTerm(n)[0]
	}
	return att
}

func expandNodeSelectorTerms(l []interface{}) []corev1.NodeSelectorTerm {
	if len(l) == 0 || l[0] == nil {
		return []corev1.NodeSelectorTerm{}
	}
	obj := make([]corev1.NodeSelectorTerm, len(l), len(l))
	for i, n := range l {
		obj[i] = *expandNodeSelectorTerm([]interface{}{n})
	}
	return obj
}
