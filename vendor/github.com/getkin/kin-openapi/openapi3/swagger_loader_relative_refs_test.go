package openapi3_test

import (
	"fmt"

	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

type refTestDataEntry struct {
	name            string
	contentTemplate string
	testFunc        func(t *testing.T, swagger *openapi3.Swagger)
}

type refTestDataEntryWithErrorMessage struct {
	name            string
	contentTemplate string
	errorMessage    *string
	testFunc        func(t *testing.T, swagger *openapi3.Swagger)
}

var refTestDataEntries = []refTestDataEntry{
	{
		name:            "SchemaRef",
		contentTemplate: externalSchemaRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Schemas["TestSchema"].Value.Type)
			require.Equal(t, "string", swagger.Components.Schemas["TestSchema"].Value.Type)
		},
	},
	{
		name:            "ResponseRef",
		contentTemplate: externalResponseRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Responses["TestResponse"].Value.Description)
			require.Equal(t, "description", swagger.Components.Responses["TestResponse"].Value.Description)
		},
	},
	{
		name:            "ParameterRef",
		contentTemplate: externalParameterRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Parameters["TestParameter"].Value.Name)
			require.Equal(t, "id", swagger.Components.Parameters["TestParameter"].Value.Name)
		},
	},
	{
		name:            "ExampleRef",
		contentTemplate: externalExampleRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Examples["TestExample"].Value.Description)
			require.Equal(t, "description", swagger.Components.Examples["TestExample"].Value.Description)
		},
	},
	{
		name:            "RequestBodyRef",
		contentTemplate: externalRequestBodyRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.RequestBodies["TestRequestBody"].Value.Content)
		},
	},
	{
		name:            "SecuritySchemeRef",
		contentTemplate: externalSecuritySchemeRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.SecuritySchemes["TestSecurityScheme"].Value.Description)
			require.Equal(t, "description", swagger.Components.SecuritySchemes["TestSecurityScheme"].Value.Description)
		},
	},
	{
		name:            "ExternalHeaderRef",
		contentTemplate: externalHeaderRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Headers["TestHeader"].Value.Description)
			require.Equal(t, "description", swagger.Components.Headers["TestHeader"].Value.Description)
		},
	},
	{
		name:            "PathParameterRef",
		contentTemplate: externalPathParameterRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test/{id}"].Parameters[0].Value.Name)
			require.Equal(t, "id", swagger.Paths["/test/{id}"].Parameters[0].Value.Name)
		},
	},
	{
		name:            "PathOperationParameterRef",
		contentTemplate: externalPathOperationParameterRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test/{id}"].Get.Parameters[0].Value)
			require.Equal(t, "id", swagger.Paths["/test/{id}"].Get.Parameters[0].Value.Name)
		},
	},
	{
		name:            "PathOperationRequestBodyRef",
		contentTemplate: externalPathOperationRequestBodyRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.RequestBody.Value)
			require.NotNil(t, swagger.Paths["/test"].Post.RequestBody.Value.Content)
		},
	},
	{
		name:            "PathOperationResponseRef",
		contentTemplate: externalPathOperationResponseRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.Responses["default"].Value)
			require.Equal(t, "description", swagger.Paths["/test"].Post.Responses["default"].Value.Description)
		},
	},
	{
		name:            "PathOperationParameterSchemaRef",
		contentTemplate: externalPathOperationParameterSchemaRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test/{id}"].Get.Parameters[0].Value.Schema.Value)
			require.Equal(t, "string", swagger.Paths["/test/{id}"].Get.Parameters[0].Value.Schema.Value.Type)
			require.Equal(t, "id", swagger.Paths["/test/{id}"].Get.Parameters[0].Value.Name)
		},
	},

	{
		name:            "PathOperationParameterRefWithContentInQuery",
		contentTemplate: externalPathOperationParameterWithContentInQueryTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			schemaRef := swagger.Paths["/test/{id}"].Get.Parameters[0].Value.Content["application/json"].Schema
			require.NotNil(t, schemaRef.Value)
			require.Equal(t, "string", schemaRef.Value.Type)
		},
	},

	{
		name:            "PathOperationRequestBodyExampleRef",
		contentTemplate: externalPathOperationRequestBodyExampleRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.RequestBody.Value.Content["application/json"].Examples["application/json"].Value)
			require.Equal(t, "description", swagger.Paths["/test"].Post.RequestBody.Value.Content["application/json"].Examples["application/json"].Value.Description)
		},
	},
	{
		name:            "PathOperationReqestBodyContentSchemaRef",
		contentTemplate: externalPathOperationReqestBodyContentSchemaRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.RequestBody.Value.Content["application/json"].Schema.Value)
			require.Equal(t, "string", swagger.Paths["/test"].Post.RequestBody.Value.Content["application/json"].Schema.Value.Type)
		},
	},
	{
		name:            "PathOperationResponseExampleRef",
		contentTemplate: externalPathOperationResponseExampleRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.Responses["default"].Value)
			require.Equal(t, "testdescription", swagger.Paths["/test"].Post.Responses["default"].Value.Description)
			require.Equal(t, "description", swagger.Paths["/test"].Post.Responses["default"].Value.Content["application/json"].Examples["application/json"].Value.Description)
		},
	},
	{
		name:            "PathOperationResponseSchemaRef",
		contentTemplate: externalPathOperationResponseSchemaRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.Responses["default"].Value)
			require.Equal(t, "testdescription", swagger.Paths["/test"].Post.Responses["default"].Value.Description)
			require.Equal(t, "string", swagger.Paths["/test"].Post.Responses["default"].Value.Content["application/json"].Schema.Value.Type)
		},
	},
	{
		name:            "ComponentHeaderSchemaRef",
		contentTemplate: externalComponentHeaderSchemaRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Components.Headers["TestHeader"].Value)
			require.Equal(t, "string", swagger.Components.Headers["TestHeader"].Value.Schema.Value.Type)
		},
	},
	{
		name:            "RequestResponseHeaderRef",
		contentTemplate: externalRequestResponseHeaderRefTemplate,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
			require.NotNil(t, swagger.Paths["/test"].Post.Responses["default"].Value.Headers["X-TEST-HEADER"].Value.Description)
			require.Equal(t, "description", swagger.Paths["/test"].Post.Responses["default"].Value.Headers["X-TEST-HEADER"].Value.Description)
		},
	},
}

var refTestDataEntriesResponseError = []refTestDataEntryWithErrorMessage{
	{
		name:            "CannotContainBothSchemaAndContentInAParameter",
		contentTemplate: externalCannotContainBothSchemaAndContentInAParameter,
		errorMessage:    &(&struct{ x string }{"Cannot contain both schema and content in a parameter"}).x,
		testFunc: func(t *testing.T, swagger *openapi3.Swagger) {
		},
	},
}

func TestLoadFromDataWithExternalRef(t *testing.T) {
	for _, td := range refTestDataEntries {
		t.Logf("testcase '%s'", td.name)

		spec := []byte(fmt.Sprintf(td.contentTemplate, "components.openapi.json"))
		loader := openapi3.NewSwaggerLoader()
		loader.IsExternalRefsAllowed = true
		swagger, err := loader.LoadSwaggerFromDataWithPath(spec, &url.URL{Path: "testdata/testfilename.openapi.json"})
		require.NoError(t, err)
		td.testFunc(t, swagger)
	}
}

func TestLoadFromDataWithExternalRefResponseError(t *testing.T) {
	for _, td := range refTestDataEntriesResponseError {
		t.Logf("testcase '%s'", td.name)

		spec := []byte(fmt.Sprintf(td.contentTemplate, "components.openapi.json"))
		loader := openapi3.NewSwaggerLoader()
		loader.IsExternalRefsAllowed = true
		swagger, err := loader.LoadSwaggerFromDataWithPath(spec, &url.URL{Path: "testdata/testfilename.openapi.json"})
		require.EqualError(t, err, *td.errorMessage)
		td.testFunc(t, swagger)
	}
}

func TestLoadFromDataWithExternalNestedRef(t *testing.T) {
	for _, td := range refTestDataEntries {
		t.Logf("testcase '%s'", td.name)

		spec := []byte(fmt.Sprintf(td.contentTemplate, "nesteddir/nestedcomponents.openapi.json"))
		loader := openapi3.NewSwaggerLoader()
		loader.IsExternalRefsAllowed = true
		swagger, err := loader.LoadSwaggerFromDataWithPath(spec, &url.URL{Path: "testdata/testfilename.openapi.json"})
		require.NoError(t, err)
		td.testFunc(t, swagger)
	}
}

const externalSchemaRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "schemas": {
            "TestSchema": {
                "$ref": "%s#/components/schemas/CustomTestSchema"
            }
        }
    }
}`

const externalResponseRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "responses": {
            "TestResponse": {
                "$ref": "%s#/components/responses/CustomTestResponse"
            }
        }
    }
}`

const externalParameterRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "parameters": {
            "TestParameter": {
                "$ref": "%s#/components/parameters/CustomTestParameter"
            }
        }
    }
}`

const externalExampleRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "examples": {
            "TestExample": {
                "$ref": "%s#/components/examples/CustomTestExample"
            }
        }
    }
}`

const externalRequestBodyRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "requestBodies": {
            "TestRequestBody": {
                "$ref": "%s#/components/requestBodies/CustomTestRequestBody"
            }
        }
    }
}`

const externalSecuritySchemeRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "securitySchemes": {
            "TestSecurityScheme": {
                "$ref": "%s#/components/securitySchemes/CustomTestSecurityScheme"
            }
        }
    }
}`

const externalHeaderRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "headers": {
            "TestHeader": {
                "$ref": "%s#/components/headers/CustomTestHeader"
            }
        }
    }
}`

const externalPathParameterRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test/{id}": {
            "parameters": [
                {
                    "$ref": "%s#/components/parameters/CustomTestParameter"
                }
            ]
        }
    }
}`

const externalPathOperationParameterRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test/{id}": {
            "get": {
                "responses": {},
                "parameters": [
                    {
                        "$ref": "%s#/components/parameters/CustomTestParameter"
                    }
                ]
            }
        }
    }
}`

const externalPathOperationRequestBodyRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {},
                "requestBody": {
                    "$ref": "%s#/components/requestBodies/CustomTestRequestBody"
                }
            }
        }
    }
}`

const externalPathOperationResponseRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {
                    "default": {
                        "$ref": "%s#/components/responses/CustomTestResponse"
                    }
                }
            }
        }
    }
}`

const externalPathOperationParameterSchemaRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test/{id}": {
            "get": {
                "responses": {},
                "parameters": [
                    {
                        "$ref": "#/components/parameters/CustomTestParameter"
                    }
                ]
            }
        }
    },
    "components": {
        "parameters": {
            "CustomTestParameter": {
                "name": "id",
                "in": "header",
                "schema": {
                    "$ref": "%s#/components/schemas/CustomTestSchema"
                }
            }
        }
    }
}`

const externalPathOperationParameterWithContentInQueryTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test/{id}": {
            "get": {
                "responses": {},
                "parameters": [
                    {
                        "$ref": "#/components/parameters/CustomTestParameter"
                    }
                ]
            }
        }
    },
    "components": {
        "parameters": {
            "CustomTestParameter": {
                "content": {
                    "application/json": {
                        "schema": {
                            "$ref": "%s#/components/schemas/CustomTestSchema"
                        }
                    }
                }
            }
        }
    }
}`

const externalCannotContainBothSchemaAndContentInAParameter = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test/{id}": {
            "get": {
                "responses": {},
                "parameters": [
                    {
                        "$ref": "#/components/parameters/CustomTestParameter"
                    }
                ]
            }
        }
    },
    "components": {
        "parameters": {
            "CustomTestParameter": {
                "content": {
                    "application/json": {
                        "schema": {
                            "$ref": "%s#/components/schemas/CustomTestSchema"
                        }
                    }
                },
                "schema": {
                    "$ref": "%s#/components/schemas/CustomTestSchema"
                }
            }
        }
    }
}`

const externalPathOperationRequestBodyExampleRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {},
                "requestBody": {
                    "$ref": "#/components/requestBodies/CustomTestRequestBody"
                }
            }
        }
    },
    "components": {
        "requestBodies": {
            "CustomTestRequestBody": {
                "content": {
                    "application/json": {
                        "examples": {
                            "application/json": {
                                "$ref": "%s#/components/examples/CustomTestExample"
                            }
                        }
                    }
                }
            }
        }
    }
}`

const externalPathOperationReqestBodyContentSchemaRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {},
                "requestBody": {
                    "$ref": "#/components/requestBodies/CustomTestRequestBody"
                }
            }
        }
    },
    "components": {
        "requestBodies": {
            "CustomTestRequestBody": {
                "content": {
                    "application/json": {
                        "schema": {
                            "$ref": "%s#/components/schemas/CustomTestSchema"
                        }
                    }
                }
            }
        }
    }
}`

const externalPathOperationResponseExampleRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {
                    "default": {
                        "$ref": "#/components/responses/CustomTestResponse"
                    }
                }
            }
        }
    },
    "components": {
        "responses": {
            "CustomTestResponse": {
                "description": "testdescription",
                "content": {
                    "application/json": {
                        "examples": {
                            "application/json": {
                                "$ref": "%s#/components/examples/CustomTestExample"
                            }
                        }
                    }
                }
            }
        }
    }
}`

const externalPathOperationResponseSchemaRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {
                    "default": {
                        "$ref": "#/components/responses/CustomTestResponse"
                    }
                }
            }
        }
    },
    "components": {
        "responses": {
            "CustomTestResponse": {
                "description": "testdescription",
                "content": {
                    "application/json": {
                        "schema": {
                            "$ref": "%s#/components/schemas/CustomTestSchema"
                        }
                    }
                }
            }
        }
    }
}`

const externalComponentHeaderSchemaRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {},
    "components": {
        "headers": {
            "TestHeader": {
                "$ref": "#/components/headers/CustomTestHeader"
            },
            "CustomTestHeader": {
                "name": "X-TEST-HEADER",
                "in": "header",
                "schema": {
                    "$ref": "%s#/components/schemas/CustomTestSchema"
                }
            }
        }
    }
}`

const externalRequestResponseHeaderRefTemplate = `
{
    "openapi": "3.0.0",
    "info": {
        "title": "",
        "version": "1"
    },
    "paths": {
        "/test": {
            "post": {
                "responses": {
                    "default": {
                        "description": "test",
                        "headers": {
                            "X-TEST-HEADER": {
                                "$ref": "%s#/components/headers/CustomTestHeader"
                            }
                        }
                    }
                }
            }
        }
    }
}`
