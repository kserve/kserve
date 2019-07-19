package openapi3filter

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ParseErrorKind describes a kind of ParseError.
// The type simplifies comparison of errors.
type ParseErrorKind int

const (
	// KindOther describes an untyped parsing error.
	KindOther ParseErrorKind = iota
	// KindUnsupportedFormat describes an error that happens when a value has an unsupported format.
	KindUnsupportedFormat
	// KindInvalidFormat describes an error that happens when a value does not conform a format
	// that is required by a serialization method.
	KindInvalidFormat
)

// ParseError describes errors which happens while parse operation's parameters, requestBody, or response.
type ParseError struct {
	Kind   ParseErrorKind
	Value  interface{}
	Reason string
	Cause  error

	path []interface{}
}

func (e *ParseError) Error() string {
	var msg []string
	if p := e.Path(); len(p) > 0 {
		var arr []string
		for _, v := range p {
			arr = append(arr, fmt.Sprintf("%v", v))
		}
		msg = append(msg, fmt.Sprintf("path %v", strings.Join(arr, ".")))
	}
	msg = append(msg, e.innerError())
	return strings.Join(msg, ": ")
}

func (e *ParseError) innerError() string {
	var msg []string
	if e.Value != nil {
		msg = append(msg, fmt.Sprintf("value %v", e.Value))
	}
	if e.Reason != "" {
		msg = append(msg, e.Reason)
	}
	if e.Cause != nil {
		if v, ok := e.Cause.(*ParseError); ok {
			msg = append(msg, v.innerError())
		} else {
			msg = append(msg, e.Cause.Error())
		}
	}
	return strings.Join(msg, ": ")
}

// RootCause returns a root cause of ParseError.
func (e *ParseError) RootCause() error {
	if v, ok := e.Cause.(*ParseError); ok {
		return v.RootCause()
	}
	return e.Cause
}

// Path returns a path to the root cause.
func (e *ParseError) Path() []interface{} {
	var path []interface{}
	if v, ok := e.Cause.(*ParseError); ok {
		p := v.Path()
		if len(p) > 0 {
			path = append(path, p...)
		}
	}
	if len(e.path) > 0 {
		path = append(path, e.path...)
	}
	return path
}

func invalidSerializationMethodErr(sm *openapi3.SerializationMethod) error {
	return fmt.Errorf("invalid serialization method: style=%q, explode=%v", sm.Style, sm.Explode)
}

// Decodes a parameter defined via the content property as an object. It uses
// the user specified decoder, or our build-in decoder for application/json
func decodeContentParameter(param *openapi3.Parameter, input *RequestValidationInput) (
	value interface{}, schema *openapi3.Schema, err error) {

	paramValues := make([]string, 1)
	var found bool
	switch param.In {
	case openapi3.ParameterInPath:
		paramValues[0], found = input.PathParams[param.Name]
	case openapi3.ParameterInQuery:
		paramValues, found = input.GetQueryParams()[param.Name]
	case openapi3.ParameterInHeader:
		paramValues[0] = input.Request.Header.Get(http.CanonicalHeaderKey(param.Name))
		found = paramValues[0] != ""
	case openapi3.ParameterInCookie:
		var cookie *http.Cookie
		cookie, err = input.Request.Cookie(param.Name)
		if err == http.ErrNoCookie {
			found = false
		} else if err != nil {
			return
		} else {
			paramValues[0] = cookie.Value
			found = true
		}
	default:
		err = fmt.Errorf("unsupported parameter's 'in': %s", param.In)
		return
	}

	if !found {
		if param.Required {
			err = fmt.Errorf("parameter '%s' is required, but missing", param.Name)
		}
		return
	}

	decoder := input.ParamDecoder
	if decoder == nil {
		decoder = defaultContentParameterDecoder
	}

	value, schema, err = decoder(param, paramValues)
	return
}

func defaultContentParameterDecoder(param *openapi3.Parameter, values []string) (
	outValue interface{}, outSchema *openapi3.Schema, err error) {
	// Only query parameters can have multiple values.
	if len(values) > 1 && param.In != openapi3.ParameterInQuery {
		err = fmt.Errorf("%s parameter '%s' can't have multiple values", param.In, param.Name)
		return
	}

	content := param.Content
	if content == nil {
		err = fmt.Errorf("parameter '%s' expected to have content", param.Name)
		return
	}

	// We only know how to decode a parameter if it has one content, application/json
	if len(content) != 1 {
		err = fmt.Errorf("multiple content types for parameter '%s'",
			param.Name)
		return
	}

	mt := content.Get("application/json")
	if mt == nil {
		err = fmt.Errorf("parameter '%s' has no json content schema", param.Name)
		return
	}
	outSchema = mt.Schema.Value

	if len(values) == 1 {
		err = json.Unmarshal([]byte(values[0]), &outValue)
		if err != nil {
			err = fmt.Errorf("error unmarshaling parameter '%s' as json", param.Name)
			return
		}
	} else {
		outArray := make([]interface{}, len(values))
		for i, v := range values {
			err = json.Unmarshal([]byte(v), &outArray[i])
			if err != nil {
				err = fmt.Errorf("error unmarshaling parameter '%s' as json", param.Name)
				return
			}
		}
		outValue = outArray
	}
	return
}

type valueDecoder interface {
	DecodePrimitive(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error)
	DecodeArray(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) ([]interface{}, error)
	DecodeObject(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (map[string]interface{}, error)
}

// decodeStyledParameter returns a value of an operation's parameter from HTTP request for
// parameters defined using the style format.
// The function returns ParseError when HTTP request contains an invalid value of a parameter.
func decodeStyledParameter(param *openapi3.Parameter, input *RequestValidationInput) (interface{}, error) {
	sm, err := param.SerializationMethod()
	if err != nil {
		return nil, err
	}

	var dec valueDecoder
	switch param.In {
	case openapi3.ParameterInPath:
		dec = &pathParamDecoder{pathParams: input.PathParams}
	case openapi3.ParameterInQuery:
		dec = &urlValuesDecoder{values: input.GetQueryParams()}
	case openapi3.ParameterInHeader:
		dec = &headerParamDecoder{header: input.Request.Header}
	case openapi3.ParameterInCookie:
		dec = &cookieParamDecoder{req: input.Request}
	default:
		return nil, fmt.Errorf("unsupported parameter's 'in': %s", param.In)
	}

	return decodeValue(dec, param.Name, sm, param.Schema)
}

func decodeValue(dec valueDecoder, param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
	var decodeFn func(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error)
	switch schema.Value.Type {
	case "array":
		decodeFn = func(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
			return dec.DecodeArray(param, sm, schema)
		}
	case "object":
		decodeFn = func(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
			return dec.DecodeObject(param, sm, schema)
		}
	default:
		decodeFn = dec.DecodePrimitive
	}

	return decodeFn(param, sm, schema)
}

// pathParamDecoder decodes values of path parameters.
type pathParamDecoder struct {
	pathParams map[string]string
}

func (d *pathParamDecoder) DecodePrimitive(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
	var prefix string
	switch sm.Style {
	case "simple":
		// A prefix is empty for style "simple".
	case "label":
		prefix = "."
	case "matrix":
		prefix = ";" + param + "="
	default:
		return nil, invalidSerializationMethodErr(sm)
	}

	if d.pathParams == nil {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	raw, ok := d.pathParams[paramKey(param, sm)]
	if !ok || raw == "" {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	src, err := cutPrefix(raw, prefix)
	if err != nil {
		return nil, err
	}
	return parsePrimitive(src, schema)
}

func (d *pathParamDecoder) DecodeArray(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) ([]interface{}, error) {
	var prefix, delim string
	switch {
	case sm.Style == "simple":
		delim = ","
	case sm.Style == "label" && sm.Explode == false:
		prefix = "."
		delim = ","
	case sm.Style == "label" && sm.Explode == true:
		prefix = "."
		delim = "."
	case sm.Style == "matrix" && sm.Explode == false:
		prefix = ";" + param + "="
		delim = ","
	case sm.Style == "matrix" && sm.Explode == true:
		prefix = ";" + param + "="
		delim = ";" + param + "="
	default:
		return nil, invalidSerializationMethodErr(sm)
	}

	if d.pathParams == nil {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	raw, ok := d.pathParams[paramKey(param, sm)]
	if !ok || raw == "" {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	src, err := cutPrefix(raw, prefix)
	if err != nil {
		return nil, err
	}
	return parseArray(strings.Split(src, delim), schema)
}

func (d *pathParamDecoder) DecodeObject(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	var prefix, propsDelim, valueDelim string
	switch {
	case sm.Style == "simple" && sm.Explode == false:
		propsDelim = ","
		valueDelim = ","
	case sm.Style == "simple" && sm.Explode == true:
		propsDelim = ","
		valueDelim = "="
	case sm.Style == "label" && sm.Explode == false:
		prefix = "."
		propsDelim = ","
		valueDelim = ","
	case sm.Style == "label" && sm.Explode == true:
		prefix = "."
		propsDelim = "."
		valueDelim = "="
	case sm.Style == "matrix" && sm.Explode == false:
		prefix = ";" + param + "="
		propsDelim = ","
		valueDelim = ","
	case sm.Style == "matrix" && sm.Explode == true:
		prefix = ";"
		propsDelim = ";"
		valueDelim = "="
	default:
		return nil, invalidSerializationMethodErr(sm)
	}

	if d.pathParams == nil {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	raw, ok := d.pathParams[paramKey(param, sm)]
	if !ok || raw == "" {
		// HTTP request does not contains a value of the target path parameter.
		return nil, nil
	}
	src, err := cutPrefix(raw, prefix)
	if err != nil {
		return nil, err
	}
	props, err := propsFromString(src, propsDelim, valueDelim)
	if err != nil {
		return nil, err
	}
	return makeObject(props, schema)
}

// paramKey returns a key to get a raw value of a path parameter.
func paramKey(param string, sm *openapi3.SerializationMethod) string {
	switch sm.Style {
	case "label":
		return "." + param
	case "matrix":
		return ";" + param
	default:
		return param
	}
}

// cutPrefix validates that a raw value of a path parameter has the specified prefix,
// and returns a raw value without the prefix.
func cutPrefix(raw, prefix string) (string, error) {
	if prefix == "" {
		return raw, nil
	}
	if len(raw) < len(prefix) || raw[:len(prefix)] != prefix {
		return "", &ParseError{
			Kind:   KindInvalidFormat,
			Value:  raw,
			Reason: fmt.Sprintf("a value must be prefixed with %q", prefix),
		}
	}
	return raw[len(prefix):], nil
}

// urlValuesDecoder decodes values of query parameters.
type urlValuesDecoder struct {
	values url.Values
}

func (d *urlValuesDecoder) DecodePrimitive(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
	if sm.Style != "form" {
		return nil, invalidSerializationMethodErr(sm)
	}

	values := d.values[param]
	if len(values) == 0 {
		// HTTP request does not contain a value of the target query parameter.
		return nil, nil
	}
	return parsePrimitive(values[0], schema)
}

func (d *urlValuesDecoder) DecodeArray(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) ([]interface{}, error) {
	if sm.Style == "deepObject" {
		return nil, invalidSerializationMethodErr(sm)
	}

	values := d.values[param]
	if len(values) == 0 {
		// HTTP request does not contain a value of the target query parameter.
		return nil, nil
	}
	if !sm.Explode {
		var delim string
		switch sm.Style {
		case "form":
			delim = ","
		case "spaceDelimited":
			delim = " "
		case "pipeDelimited":
			delim = "|"
		}
		values = strings.Split(values[0], delim)
	}
	return parseArray(values, schema)
}

func (d *urlValuesDecoder) DecodeObject(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	var propsFn func(url.Values) (map[string]string, error)
	switch sm.Style {
	case "form":
		propsFn = func(params url.Values) (map[string]string, error) {
			if len(params) == 0 {
				// HTTP request does not contain query parameters.
				return nil, nil
			}
			if sm.Explode {
				props := make(map[string]string)
				for key, values := range params {
					props[key] = values[0]
				}
				return props, nil
			}
			values := params[param]
			if len(values) == 0 {
				// HTTP request does not contain a value of the target query parameter.
				return nil, nil
			}
			return propsFromString(values[0], ",", ",")
		}
	case "deepObject":
		propsFn = func(params url.Values) (map[string]string, error) {
			props := make(map[string]string)
			for key, values := range params {
				groups := regexp.MustCompile(fmt.Sprintf("%s\\[(.+?)\\]", param)).FindAllStringSubmatch(key, -1)
				if len(groups) == 0 {
					// A query parameter's name does not match the required format, so skip it.
					continue
				}
				props[groups[0][1]] = values[0]
			}
			if len(props) == 0 {
				// HTTP request does not contain query parameters encoded by rules of style "deepObject".
				return nil, nil
			}
			return props, nil
		}
	default:
		return nil, invalidSerializationMethodErr(sm)
	}

	props, err := propsFn(d.values)
	if err != nil {
		return nil, err
	}
	if props == nil {
		return nil, nil
	}
	return makeObject(props, schema)
}

// headerParamDecoder decodes values of header parameters.
type headerParamDecoder struct {
	header http.Header
}

func (d *headerParamDecoder) DecodePrimitive(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
	if sm.Style != "simple" {
		return nil, invalidSerializationMethodErr(sm)
	}

	raw := d.header.Get(http.CanonicalHeaderKey(param))
	return parsePrimitive(raw, schema)
}

func (d *headerParamDecoder) DecodeArray(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) ([]interface{}, error) {
	if sm.Style != "simple" {
		return nil, invalidSerializationMethodErr(sm)
	}

	raw := d.header.Get(http.CanonicalHeaderKey(param))
	if raw == "" {
		// HTTP request does not contains a corresponding header
		return nil, nil
	}
	return parseArray(strings.Split(raw, ","), schema)
}

func (d *headerParamDecoder) DecodeObject(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	if sm.Style != "simple" {
		return nil, invalidSerializationMethodErr(sm)
	}
	valueDelim := ","
	if sm.Explode {
		valueDelim = "="
	}

	raw := d.header.Get(http.CanonicalHeaderKey(param))
	if raw == "" {
		// HTTP request does not contain a corresponding header.
		return nil, nil
	}
	props, err := propsFromString(raw, ",", valueDelim)
	if err != nil {
		return nil, err
	}
	return makeObject(props, schema)
}

// cookieParamDecoder decodes values of cookie parameters.
type cookieParamDecoder struct {
	req *http.Request
}

func (d *cookieParamDecoder) DecodePrimitive(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (interface{}, error) {
	if sm.Style != "form" {
		return nil, invalidSerializationMethodErr(sm)
	}

	cookie, err := d.req.Cookie(param)
	if err == http.ErrNoCookie {
		// HTTP request does not contain a corresponding cookie.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode param %q: %s", param, err)
	}
	return parsePrimitive(cookie.Value, schema)
}

func (d *cookieParamDecoder) DecodeArray(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) ([]interface{}, error) {
	if sm.Style != "form" || sm.Explode {
		return nil, invalidSerializationMethodErr(sm)
	}

	cookie, err := d.req.Cookie(param)
	if err == http.ErrNoCookie {
		// HTTP request does not contain a corresponding cookie.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode param %q: %s", param, err)
	}
	return parseArray(strings.Split(cookie.Value, ","), schema)
}

func (d *cookieParamDecoder) DecodeObject(param string, sm *openapi3.SerializationMethod, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	if sm.Style != "form" || sm.Explode {
		return nil, invalidSerializationMethodErr(sm)
	}

	cookie, err := d.req.Cookie(param)
	if err == http.ErrNoCookie {
		// HTTP request does not contain a corresponding cookie.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode param %q: %s", param, err)
	}
	props, err := propsFromString(cookie.Value, ",", ",")
	if err != nil {
		return nil, err
	}
	return makeObject(props, schema)
}

// propsFromString returns a properties map that is created by splitting a source string by propDelim and valueDelim.
// The source string must have a valid format: pairs <propName><valueDelim><propValue> separated by <propDelim>.
// The function returns an error when the source string has an invalid format.
func propsFromString(src, propDelim, valueDelim string) (map[string]string, error) {
	props := make(map[string]string)
	pairs := strings.Split(src, propDelim)

	// When propDelim and valueDelim is equal the source string follow the next rule:
	// every even item of pairs is a properies's name, and the subsequent odd item is a property's value.
	if propDelim == valueDelim {
		// Taking into account the rule above, a valid source string must be splitted by propDelim
		// to an array with an even number of items.
		if len(pairs)%2 != 0 {
			return nil, &ParseError{
				Kind:   KindInvalidFormat,
				Value:  src,
				Reason: fmt.Sprintf("a value must be a list of object's properties in format \"name%svalue\" separated by %s", valueDelim, propDelim),
			}
		}
		for i := 0; i < len(pairs)/2; i++ {
			props[pairs[i*2]] = pairs[i*2+1]
		}
		return props, nil
	}

	// When propDelim and valueDelim is not equal the source string follow the next rule:
	// every item of pairs is a string that follows format <propName><valueDelim><propValue>.
	for _, pair := range pairs {
		prop := strings.Split(pair, valueDelim)
		if len(prop) != 2 {
			return nil, &ParseError{
				Kind:   KindInvalidFormat,
				Value:  src,
				Reason: fmt.Sprintf("a value must be a list of object's properties in format \"name%svalue\" separated by %s", valueDelim, propDelim),
			}
		}
		props[prop[0]] = prop[1]
	}
	return props, nil
}

// makeObject returns an object that contains properties from props.
// A value of every property is parsed as a primitive value.
// The function returns an error when an error happened while parse object's properties.
func makeObject(props map[string]string, schema *openapi3.SchemaRef) (map[string]interface{}, error) {
	obj := make(map[string]interface{})
	for propName, propSchema := range schema.Value.Properties {
		value, err := parsePrimitive(props[propName], propSchema)
		if err != nil {
			if v, ok := err.(*ParseError); ok {
				return nil, &ParseError{path: []interface{}{propName}, Cause: v}
			}
			return nil, fmt.Errorf("property %q: %s", propName, err)
		}
		obj[propName] = value
	}
	return obj, nil
}

// parseArray returns an array that contains items from a raw array.
// Every item is parsed as a primitive value.
// The function returns an error when an error happened while parse array's items.
func parseArray(raw []string, schemaRef *openapi3.SchemaRef) ([]interface{}, error) {
	var value []interface{}
	for i, v := range raw {
		item, err := parsePrimitive(v, schemaRef.Value.Items)
		if err != nil {
			if v, ok := err.(*ParseError); ok {
				return nil, &ParseError{path: []interface{}{i}, Cause: v}
			}
			return nil, fmt.Errorf("item %d: %s", i, err)
		}
		value = append(value, item)
	}
	return value, nil
}

// parsePrimitive returns a value that is created by parsing a source string to a primitive type
// that is specified by a JSON schema. The function returns nil when the source string is empty.
// The function panics when a JSON schema has a non primitive type.
func parsePrimitive(raw string, schema *openapi3.SchemaRef) (interface{}, error) {
	if raw == "" {
		return nil, nil
	}
	switch schema.Value.Type {
	case "integer":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, &ParseError{Kind: KindInvalidFormat, Value: raw, Reason: "an invalid interger", Cause: err}
		}
		return v, nil
	case "number":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, &ParseError{Kind: KindInvalidFormat, Value: raw, Reason: "an invalid number", Cause: err}
		}
		return v, nil
	case "boolean":
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, &ParseError{Kind: KindInvalidFormat, Value: raw, Reason: "an invalid number", Cause: err}
		}
		return v, nil
	case "string":
		return raw, nil
	default:
		panic(fmt.Sprintf("schema has non primitive type %q", schema.Value.Type))
	}
}

// EncodingFn is a function that returns an encoding of a request body's part.
type EncodingFn func(partName string) *openapi3.Encoding

// BodyDecoder is an interface to decode a body of a request or response.
// An implementation must return a value that is a primitive, []interface{}, or map[string]interface{}.
type BodyDecoder func(io.Reader, http.Header, *openapi3.SchemaRef, EncodingFn) (interface{}, error)

// bodyDecoders contains decoders for supported content types of a body.
// By default, there is content type "application/json" is supported only.
var bodyDecoders = make(map[string]BodyDecoder)

// RegisterBodyDecoder registers a request body's decoder for a content type.
//
// If a decoder for the specified content type already exists, the function replaces
// it with the specified decoder.
func RegisterBodyDecoder(contentType string, decoder BodyDecoder) {
	if contentType == "" {
		panic("contentType is empty")
	}
	if decoder == nil {
		panic("decoder is not defined")
	}
	bodyDecoders[contentType] = decoder
}

// UnregisterBodyDecoder dissociates a body decoder from a content type.
//
// Decoding this content type will result in an error.
func UnregisterBodyDecoder(contentType string) {
	if contentType == "" {
		panic("contentType is empty")
	}
	delete(bodyDecoders, contentType)
}

// decodeBody returns a decoded body.
// The function returns ParseError when a body is invalid.
func decodeBody(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	contentType := header.Get(http.CanonicalHeaderKey("Content-Type"))
	mediaType := parseMediaType(contentType)
	decoder, ok := bodyDecoders[mediaType]
	if !ok {
		return nil, &ParseError{
			Kind:   KindUnsupportedFormat,
			Reason: fmt.Sprintf("unsupported content type %q", mediaType),
		}
	}
	value, err := decoder(body, header, schema, encFn)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func init() {
	RegisterBodyDecoder("text/plain", plainBodyDecoder)
	RegisterBodyDecoder("application/json", jsonBodyDecoder)
	RegisterBodyDecoder("application/x-www-form-urlencoded", urlencodedBodyDecoder)
	RegisterBodyDecoder("multipart/form-data", multipartBodyDecoder)
	RegisterBodyDecoder("application/octet-stream", FileBodyDecoder)
}

func plainBodyDecoder(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, &ParseError{Kind: KindInvalidFormat, Cause: err}
	}
	return string(data), nil
}

func jsonBodyDecoder(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	var value interface{}
	if err := json.NewDecoder(body).Decode(&value); err != nil {
		return nil, &ParseError{Kind: KindInvalidFormat, Cause: err}
	}
	return value, nil
}

func urlencodedBodyDecoder(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	// Validate JSON schema of request body.
	// By the OpenAPI 3 specification request body's schema must have type "object".
	// Properties of the schema describes individual parts of request body.
	if schema.Value.Type != "object" {
		return nil, fmt.Errorf("unsupported JSON schema of request body")
	}
	for propName, propSchema := range schema.Value.Properties {
		switch propSchema.Value.Type {
		case "object":
			return nil, fmt.Errorf("unsupported JSON schema of request body's property %q", propName)
		case "array":
			items := propSchema.Value.Items.Value
			if items.Type != "string" && items.Type != "integer" && items.Type != "number" && items.Type != "boolean" {
				return nil, fmt.Errorf("unsupported JSON schema of request body's property %q", propName)
			}
		}
	}

	// Parse form.
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(b))
	if err != nil {
		return nil, err
	}

	// Make an object value from form values.
	obj := make(map[string]interface{})
	dec := &urlValuesDecoder{values: values}
	for name, prop := range schema.Value.Properties {
		var (
			value interface{}
			enc   *openapi3.Encoding
		)
		if encFn != nil {
			enc = encFn(name)
		}
		sm := enc.SerializationMethod()

		if value, err = decodeValue(dec, name, sm, prop); err != nil {
			return nil, err
		}
		obj[name] = value
	}

	return obj, nil
}

func multipartBodyDecoder(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	if schema.Value.Type != "object" {
		return nil, fmt.Errorf("unsupported JSON schema of request body")
	}

	// Parse form.
	values := make(map[string][]interface{})
	contentType := header.Get(http.CanonicalHeaderKey("Content-Type"))
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		var part *multipart.Part
		if part, err = mr.NextPart(); err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		var (
			name = part.FormName()
			enc  *openapi3.Encoding
		)
		if encFn != nil {
			enc = encFn(name)
		}
		subEncFn := func(string) *openapi3.Encoding { return enc }
		// If the property's schema has type "array" it is means that the form contains a few parts with the same name.
		// Every such part has a type that is defined by an items schema in the property's schema.
		valueSchema := schema.Value.Properties[name]
		if valueSchema.Value.Type == "array" {
			valueSchema = valueSchema.Value.Items
		}

		var value interface{}
		if value, err = decodeBody(part, http.Header(part.Header), valueSchema, subEncFn); err != nil {
			if v, ok := err.(*ParseError); ok {
				return nil, &ParseError{path: []interface{}{name}, Cause: v}
			}
			return nil, fmt.Errorf("part %s: %s", name, err)
		}
		values[name] = append(values[name], value)
	}

	// Make an object value from form values.
	obj := make(map[string]interface{})
	for name, prop := range schema.Value.Properties {
		vv := values[name]
		if len(vv) == 0 {
			continue
		}
		if prop.Value.Type == "array" {
			obj[name] = vv
		} else {
			obj[name] = vv[0]
		}
	}

	return obj, nil
}

// FileBodyDecoder is a body decoder that decodes a file body to a string.
func FileBodyDecoder(body io.Reader, header http.Header, schema *openapi3.SchemaRef, encFn EncodingFn) (interface{}, error) {
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}
