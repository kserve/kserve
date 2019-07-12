package openapi3filter_test

import (
	"net/http"
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

func TestRouter(t *testing.T) {
	// Build swagger
	helloCONNECT := &openapi3.Operation{}
	helloDELETE := &openapi3.Operation{}
	helloGET := &openapi3.Operation{}
	helloHEAD := &openapi3.Operation{}
	helloOPTIONS := &openapi3.Operation{}
	helloPATCH := &openapi3.Operation{}
	helloPOST := &openapi3.Operation{}
	helloPUT := &openapi3.Operation{}
	helloTRACE := &openapi3.Operation{}
	paramsGET := &openapi3.Operation{}
	swagger := &openapi3.Swagger{
		Paths: openapi3.Paths{
			"/hello": &openapi3.PathItem{
				Connect: helloCONNECT,
				Delete:  helloDELETE,
				Get:     helloGET,
				Head:    helloHEAD,
				Options: helloOPTIONS,
				Patch:   helloPATCH,
				Post:    helloPOST,
				Put:     helloPUT,
				Trace:   helloTRACE,
			},
			"/params/{x}/{y}/{z*}": &openapi3.PathItem{
				Get: paramsGET,
			},
		},
	}

	// Build router
	router := openapi3filter.NewRouter().WithSwagger(swagger)

	// Declare a helper function
	expect := func(method string, uri string, operation *openapi3.Operation, params map[string]string) {
		req, err := http.NewRequest(method, uri, nil)
		if err != nil {
			panic(err)
		}
		route, pathParams, err := router.FindRoute(req.Method, req.URL)
		if err != nil {
			if operation == nil {
				return
			}
			t.Fatalf("'%s %s': should have returned an operation, but it returned an error: %v",
				method, uri, err)
		}
		if operation == nil {
			t.Fatalf("'%s %s': should have returned an error, but didn't",
				method, uri)
		}
		if route.Operation != operation {
			t.Fatalf("'%s %s': Returned wrong operation (%v)",
				method, uri, route.Operation)
		}
		if params == nil {
			if len(pathParams) != 0 {
				t.Fatalf("'%s %s': should return no path arguments, but found some",
					method, uri)
			}
		} else {
			names := make([]string, 0, len(params))
			for name := range params {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				expected := params[name]
				actual, exists := pathParams[name]
				if !exists {
					t.Fatalf("'%s %s': path parameter '%s' should be '%s', but it's not defined.",
						method, uri, name, expected)
				}
				if actual != expected {
					t.Fatalf("'%s %s': path parameter '%s' should be '%s', but it's '%s'",
						method, uri, name, expected, actual)
				}
			}
		}
	}
	expect("GET", "/not_existing", nil, nil)
	expect("DELETE", "/hello", helloDELETE, nil)
	expect("GET", "/hello", helloGET, nil)
	expect("HEAD", "/hello", helloHEAD, nil)
	expect("PATCH", "/hello", helloPATCH, nil)
	expect("POST", "/hello", helloPOST, nil)
	expect("PUT", "/hello", helloPUT, nil)
	expect("GET", "/params/a/b/c/d", paramsGET, map[string]string{
		"x": "a",
		"y": "b",
		"z": "c/d",
	})
	swagger.Servers = append(swagger.Servers, &openapi3.Server{
		URL: "https://www.example.com/api/v1/",
	}, &openapi3.Server{
		URL: "https://{d0}.{d1}.com/api/v1/",
	})
	expect("GET", "/hello", nil, nil)
	expect("GET", "/api/v1/hello", nil, nil)
	expect("GET", "www.example.com/api/v1/hello", nil, nil)
	expect("GET", "https:///api/v1/hello", nil, nil)
	expect("GET", "https://www.example.com/hello", nil, nil)
	expect("GET", "https://www.example.com/api/v1/hello", helloGET, map[string]string{})
	expect("GET", "https://domain0.domain1.com/api/v1/hello", helloGET, map[string]string{
		"d0": "domain0",
		"d1": "domain1",
	})
}
