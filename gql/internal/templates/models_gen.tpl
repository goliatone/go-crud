package model

// {{ Notice }}

{% if ModelImports and ModelImports|length > 0 %}import (
{% for imp in ModelImports %}	"{{ imp }}"
{% endfor %})

{% endif %}{% if Scalars %}{% for scalar in Scalars %}
{% if scalar.Description %}// {{ scalar.Description }}
{% endif %}type {{ scalar.Name }} {{ scalar.GoType }}
{% endfor %}{% endif %}{% if ModelEnums %}{% for enum in ModelEnums %}
type {{ enum.Name }} string

const (
{% for val in enum.Values %}	{{ enum.Name }}{{ val }} {{ enum.Name }} = "{{ val }}"
{% endfor %})

{% endfor %}{% endif %}{% for struct in ModelStructs %}
{% if struct.Description %}// {{ struct.Description }}
{% endif %}type {{ struct.Name }} struct {
{% for field in struct.Fields %}	{{ field.Name }} {{ field.GoType }} `json:"{{ field.JSONName }},omitempty"`
{% endfor %}}

{% endfor %}{% if Scalars %}
{% for scalar in Scalars %}
func Marshal{{ scalar.Name }}(v {{ scalar.Name }}) graphql.Marshaler {
{% if scalar.Name == "Time" %}	return graphql.MarshalTime(time.Time(v))
{% elif scalar.Name == "UUID" %}	return graphql.MarshalString(string(v))
{% elif scalar.Name == "JSON" %}	return graphql.MarshalAny(map[string]any(v))
{% else %}	return graphql.MarshalAny(v)
{% endif %}}

func Unmarshal{{ scalar.Name }}(v interface{}) ({{ scalar.Name }}, error) {
{% if scalar.Name == "Time" %}	t, err := graphql.UnmarshalTime(v)
	if err != nil {
		return {{ scalar.Name }}{}, err
	}
	return {{ scalar.Name }}(t), nil
{% elif scalar.Name == "UUID" %}	s, err := graphql.UnmarshalString(v)
	if err != nil {
		return {{ scalar.Name }}(""), err
	}
	return {{ scalar.Name }}(s), nil
{% elif scalar.Name == "JSON" %}	out, err := graphql.UnmarshalMap(v)
	if err != nil {
		return nil, err
	}
	return {{ scalar.Name }}(out), nil
{% else %}	var out {{ scalar.Name }}
	if err := graphql.UnmarshalAny(v, &out); err != nil {
		return {{ scalar.Name }}{}, err
	}
	return out, nil
{% endif %}}
{% endfor %}{% endif %}
