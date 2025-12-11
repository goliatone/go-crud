# {{ Notice }}

schema:
  - {{ SchemaPath }}

exec:
  filename: graph/generated/generated.go
  package: generated

model:
  filename: graph/model/gqlgen_gen.go
  package: model

resolver:
  layout: follow-schema
  dir: {{ ResolverPackage }}
  package: resolvers
  filename: {{ ResolverPackage }}/resolver_custom.go

models:{% if Scalars %}{% for scalar in Scalars %}
  {{ scalar.Name }}:
    model: {{ ModelPackage }}.{{ scalar.Name }}
{% endfor %}{% endif %}{% for entity in Entities %}
  {{ entity.Name }}:
    model: {{ ModelPackage }}.{{ entity.Name }}
{% endfor %}{% for input in Inputs %}
  {{ input.Name }}:
    model: {{ ModelPackage }}.{{ input.Name }}
{% endfor %}{% for enum in Enums %}
  {{ enum.Name }}:
    model: {{ ModelPackage }}.{{ enum.Name }}
{% endfor %}
