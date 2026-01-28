# {{ Notice }}

{% if Scalars -%}
{% for scalar in Scalars -%}
{% if scalar.Description %}"""{{ scalar.Description }}"""
{% endif %}scalar {{ scalar.Name }}
{% endfor %}
{% endif %}

{% if Enums -%}
{% for enum in Enums -%}
{% if enum.Description %}"""{{ enum.Description }}"""
{% endif %}enum {{ enum.Name }} {
{% for val in enum.Values -%}{% if val.Description %}  # {{ val.Description|safe }}
{% endif %}  {{ val.Name }}
{% endfor %}
}

{% endfor %}
{% endif %}

{% if Inputs -%}
{% for input in Inputs -%}
{% if input.Description %}"""{{ input.Description }}"""
{% endif %}input {{ input.Name }} {
{% for field in input.Fields -%}{% if field.Description %}  """{{ field.Description }}"""
{% endif %}  {{ field.Name }}: {% if field.List %}[{{ field.Type }}{% if field.Required %}!{% endif %}]{% if field.Required %}!{% endif %}{% else %}{{ field.Type }}{% if field.Required %}!{% endif %}{% endif %}
{% endfor %}
}

{% endfor %}
{% endif %}

{% if Unions -%}
{% for union in Unions -%}
union {{ union.Name }} = {% for member in union.Types %}{{ member }}{% if not forloop.last %} | {% endif %}{% endfor %}
{% endfor %}
{% endif %}

{% for entity in Entities -%}
{% if entity.Description %}"""{{ entity.Description }}"""
{% endif %}type {{ entity.Name }} {
{% for field in entity.Fields -%}
{% if field.Description %}  """{{ field.Description }}"""
{% endif %}  {{ field.Name }}: {% if field.IsList %}[{{ field.Type }}{% if field.Required and not field.Nullable %}!{% endif %}]{% if field.Required and not field.Nullable %}!{% endif %}{% else %}{{ field.Type }}{% if field.Required and not field.Nullable %}!{% endif %}{% endif %}
{% endfor %}
}

{% endfor %}

type Query {
{% for op in Queries -%}
{% if op.Description %}  """{{ op.Description }}"""
{% endif %}  {{ op.Name }}{% if op.ArgsSignature %}({{ op.ArgsSignature }}){% endif %}: {% if op.List %}[{{ op.ReturnType }}!]{% if op.Required %}!{% endif %}{% else %}{{ op.ReturnType }}{% if op.Required %}!{% endif %}{% endif %}
{% endfor -%}
}

type Mutation {
{% for op in Mutations -%}
{% if op.Description %}  """{{ op.Description }}"""
{% endif %}  {{ op.Name }}{% if op.ArgsSignature %}({{ op.ArgsSignature }}){% endif %}: {% if op.List %}[{{ op.ReturnType }}!]{% if op.Required %}!{% endif %}{% else %}{{ op.ReturnType }}{% if op.Required %}!{% endif %}{% endif %}
{% endfor -%}
}

{% if Subscriptions %}
type Subscription {
{% for op in Subscriptions -%}
{% if op.Description %}  """{{ op.Description }}"""
{% endif %}  {{ op.Name }}{% if op.ArgsSignature %}({{ op.ArgsSignature }}){% endif %}: {% if op.List %}[{{ op.ReturnType }}!]{% if op.Required %}!{% endif %}{% else %}{{ op.ReturnType }}{% if op.Required %}!{% endif %}{% endif %}
{% endfor -%}
}
{% endif %}
