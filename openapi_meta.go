package crud

import (
	"reflect"

	"github.com/goliatone/go-router"
)

var _ router.MetadataProvider = (*Controller[any])(nil)

// GetMetadata implements router.MetadataProvider, we use it
// to generate the required info that will be used to create a
// OpenAPI spec or something similar
func (c *Controller[T]) GetMetadata() router.ResourceMetadata {
	var model T
	resourceName, pluralName := GetResourceName[T]()
	modelType := reflect.TypeOf(model)

	metadata := router.ResourceMetadata{
		Name:       resourceName,
		PluralName: pluralName,
		Tags:       []string{resourceName},
		Schema:     router.ExtractSchemaFromType(modelType),
		Routes:     c.buildRoutesMetadata(),
	}

	return metadata
}

func (c *Controller[T]) buildRoutesMetadata() []router.RouteMetadata {
	resourceName, pluralName := GetResourceName[T]()

	// Common error response schema
	errorResponseSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"success": map[string]any{"type": "boolean"},
			"error":   map[string]any{"type": "string"},
		},
	}

	return []router.RouteMetadata{
		{
			Method:  "GET",
			Path:    "/" + pluralName,
			Name:    resourceName + ":list",
			Summary: "List " + pluralName,
			Tags:    []string{resourceName},
			Parameters: []router.ParameterInfo{
				{
					Name:     "limit",
					In:       "query",
					Required: false,
					Schema:   map[string]any{"type": "integer", "default": 25},
				},
				{
					Name:     "offset",
					In:       "query",
					Required: false,
					Schema:   map[string]any{"type": "integer", "default": 0},
				},
				{
					Name:        "include",
					In:          "query",
					Required:    false,
					Description: "Related resources to include, comma separated (e.g. Company,Profile)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "select",
					In:          "query",
					Required:    false,
					Description: "Fields to include in the response, comma separated (e.g. id,name,email)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "order",
					In:          "query",
					Required:    false,
					Description: "Sort order, comma separated with direction (e.g. name asc,created_at desc)",
					Schema:      map[string]any{"type": "string"},
				},
				// Field filtering parameters with operators
				{
					Name:        "{field}",
					In:          "query",
					Required:    false,
					Description: "Filter by field value (e.g. name=John)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__eq",
					In:          "query",
					Required:    false,
					Description: "Filter where field equals value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__ne",
					In:          "query",
					Required:    false,
					Description: "Filter where field does not equal value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__gt",
					In:          "query",
					Required:    false,
					Description: "Filter where field is greater than value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__lt",
					In:          "query",
					Required:    false,
					Description: "Filter where field is less than value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__gte",
					In:          "query",
					Required:    false,
					Description: "Filter where field is greater than or equal to value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__lte",
					In:          "query",
					Required:    false,
					Description: "Filter where field is less than or equal to value",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__like",
					In:          "query",
					Required:    false,
					Description: "Filter where field matches pattern (SQL LIKE)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__ilike",
					In:          "query",
					Required:    false,
					Description: "Filter where field matches pattern case insensitive (SQL ILIKE)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__and",
					In:          "query",
					Required:    false,
					Description: "Filter where field matches all values (comma separated)",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "{field}__or",
					In:          "query",
					Required:    false,
					Description: "Filter where field matches any value (comma separated)",
					Schema:      map[string]any{"type": "string"},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        200,
					Description: "Successful response",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "array",
								"items": map[string]any{
									"$ref": "#/components/schemas/" + resourceName,
								},
							},
						},
					},
				},
			},
		},
		// Get single resource by ID
		{
			Method:      "GET",
			Path:        "/" + resourceName + "/:id",
			Name:        resourceName + ":read",
			Summary:     "Get " + resourceName + " by ID",
			Tags:        []string{resourceName},
			Description: "Retrieves a single " + resourceName + " by its ID",
			Parameters: []router.ParameterInfo{
				{
					Name:        "id",
					In:          "path",
					Required:    true,
					Description: "ID of the " + resourceName,
					Schema:      map[string]any{"type": "string", "format": "uuid"},
				},
				{
					Name:        "include",
					In:          "query",
					Required:    false,
					Description: "Related resources to include, comma separated",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "select",
					In:          "query",
					Required:    false,
					Description: "Fields to include in the response, comma separated",
					Schema:      map[string]any{"type": "string"},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        200,
					Description: "Successful response",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
				{
					Code:        404,
					Description: resourceName + " not found",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Create single resource
		{
			Method:      "POST",
			Path:        "/" + resourceName,
			Name:        resourceName + ":create",
			Summary:     "Create new " + resourceName,
			Tags:        []string{resourceName},
			Description: "Creates a new " + resourceName + " record",
			RequestBody: &router.RequestBodyInfo{
				Description: "New " + resourceName + " data",
				Required:    true,
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"$ref": "#/components/schemas/" + resourceName,
						},
					},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        201,
					Description: resourceName + " created successfully",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
				{
					Code:        400,
					Description: "Invalid input",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Create batch
		{
			Method:      "POST",
			Path:        "/" + resourceName + "/batch",
			Name:        resourceName + ":create:batch",
			Summary:     "Create multiple " + pluralName,
			Tags:        []string{resourceName},
			Description: "Creates multiple " + resourceName + " records in a single request",
			RequestBody: &router.RequestBodyInfo{
				Description: "Array of new " + resourceName + " data",
				Required:    true,
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"type": "array",
							"items": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        201,
					Description: "Records created successfully",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "array",
								"items": map[string]any{
									"$ref": "#/components/schemas/" + resourceName,
								},
							},
						},
					},
				},
				{
					Code:        400,
					Description: "Invalid input",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Update single resource
		{
			Method:      "PUT",
			Path:        "/" + resourceName + "/:id",
			Name:        resourceName + ":update",
			Summary:     "Update " + resourceName + " by ID",
			Tags:        []string{resourceName},
			Description: "Updates an existing " + resourceName + " record",
			Parameters: []router.ParameterInfo{
				{
					Name:        "id",
					In:          "path",
					Required:    true,
					Description: "ID of the " + resourceName + " to update",
					Schema:      map[string]any{"type": "string", "format": "uuid"},
				},
			},
			RequestBody: &router.RequestBodyInfo{
				Description: "Updated " + resourceName + " data",
				Required:    true,
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"$ref": "#/components/schemas/" + resourceName,
						},
					},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        200,
					Description: resourceName + " updated successfully",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
				{
					Code:        404,
					Description: resourceName + " not found",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Update batch
		{
			Method:      "PUT",
			Path:        "/" + resourceName + "/batch",
			Name:        resourceName + ":update:batch",
			Summary:     "Update multiple " + pluralName,
			Tags:        []string{resourceName},
			Description: "Updates multiple " + resourceName + " records in a single request",
			RequestBody: &router.RequestBodyInfo{
				Description: "Array of " + resourceName + " updates",
				Required:    true,
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"type": "array",
							"items": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        200,
					Description: "Records updated successfully",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "array",
								"items": map[string]any{
									"$ref": "#/components/schemas/" + resourceName,
								},
							},
						},
					},
				},
				{
					Code:        400,
					Description: "Invalid input",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Delete single resource
		{
			Method:      "DELETE",
			Path:        "/" + resourceName + "/:id",
			Name:        resourceName + ":delete",
			Summary:     "Delete " + resourceName + " by ID",
			Tags:        []string{resourceName},
			Description: "Deletes a " + resourceName + " record",
			Parameters: []router.ParameterInfo{
				{
					Name:        "id",
					In:          "path",
					Required:    true,
					Description: "ID of the " + resourceName + " to delete",
					Schema:      map[string]any{"type": "string", "format": "uuid"},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        204,
					Description: resourceName + " deleted successfully",
				},
				{
					Code:        404,
					Description: resourceName + " not found",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
		// Delete batch
		{
			Method:      "DELETE",
			Path:        "/" + resourceName + "/batch",
			Name:        resourceName + ":delete:batch",
			Summary:     "Delete multiple " + pluralName,
			Tags:        []string{resourceName},
			Description: "Deletes multiple " + resourceName + " records in a single request",
			RequestBody: &router.RequestBodyInfo{
				Description: "Array of " + resourceName + " IDs to delete",
				Required:    true,
				Content: map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"type": "array",
							"items": map[string]any{
								"$ref": "#/components/schemas/" + resourceName,
							},
						},
					},
				},
			},
			Responses: []router.ResponseInfo{
				{
					Code:        204,
					Description: "Records deleted successfully",
				},
				{
					Code:        400,
					Description: "Invalid input",
					Content: map[string]any{
						"application/json": map[string]any{
							"schema": errorResponseSchema,
						},
					},
				},
			},
		},
	}
}
