/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudevents_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/knative/pkg/cloudevents"
)

type FirestoreDocument struct {
	Name       string                 `json:"name"`
	Fields     map[string]interface{} `json:"fields"`
	CreateTime time.Time              `json:"createTime"`
	UpdateTime time.Time              `json:"updateTime"`
}

var (
	webhook = "http://localhost/:sendEvent"
)

// Wrap the global functions in an HTTPMarshaller interface for table-driven testing:
type defaultMarshaller int

var Default defaultMarshaller = 0

func (defaultMarshaller) FromRequest(data interface{}, r *http.Request) (cloudevents.LoadContext, error) {
	return cloudevents.FromRequest(data, r)
}
func (defaultMarshaller) NewRequest(urlString string, data interface{}, context cloudevents.SendContext) (*http.Request, error) {
	return cloudevents.NewRequest(urlString, data, context)
}

type desiredVersion func(cloudevents.LoadContext) cloudevents.ContextType

var (
	v01 = func(a cloudevents.LoadContext) cloudevents.ContextType { tmp := a.AsV01(); return &tmp }
	v02 = func(a cloudevents.LoadContext) cloudevents.ContextType { tmp := a.AsV02(); return &tmp }
)

func TestValidRoundTrips(t *testing.T) {
	doc := FirestoreDocument{
		Name: "projects/demo/databases/default/documents/users/inlined",
		Fields: map[string]interface{}{
			"project": "eventing",
			"handle":  "@inlined",
		},
		CreateTime: time.Date(1985, 6, 5, 12, 0, 0, 0, time.UTC),
		UpdateTime: time.Now().UTC(),
	}

	service := "firestore.googleapis.com"

	encoderSets := map[string][]desiredVersion{
		"v1-v1": {v01, v01},
		"v1-v2": {v01, v02},
		"v2-v1": {v02, v01},
		"v2-v2": {v02, v02},
	}

	context := &cloudevents.EventContext{
		CloudEventsVersion: "0.1",
		EventID:            "eventid-123",
		EventTime:          doc.UpdateTime,
		EventType:          "google.firestore.document.create",
		EventTypeVersion:   "v1beta2",
		SchemaURL:          "http://type.googleapis.com/google.firestore.v1beta1.Document",
		ContentType:        "application/json",
		Source:             fmt.Sprintf("//%s/%s", service, doc.Name),
		Extensions: map[string]interface{}{
			"purpose": "tbd",
			"super":   map[string]interface{}{"cali": "fragilistic", "expi": "alidocious"},
		},
	}
	for _, test := range []struct {
		name    string
		encoder cloudevents.HTTPMarshaller
		decoder cloudevents.HTTPMarshaller
	}{
		{
			name:    "binary -> binary",
			encoder: cloudevents.Binary,
			decoder: cloudevents.Binary,
		},
		{
			name:    "binary -> default",
			encoder: cloudevents.Binary,
			decoder: Default,
		},
		{
			name:    "structured -> structured",
			encoder: cloudevents.Structured,
			decoder: cloudevents.Structured,
		},
		{
			name:    "structured -> default",
			encoder: cloudevents.Structured,
			decoder: Default,
		},
	} {
		for encoding, convert := range encoderSets {
			testName := test.name + "-" + encoding
			t.Run(testName, func(t *testing.T) {
				req, err := test.encoder.NewRequest(webhook, doc, convert[0](context))
				if err != nil {
					t.Fatalf("Failed to encode event %s", err)
				}

				var foundData FirestoreDocument
				foundContext, err := test.decoder.FromRequest(&foundData, req)
				if err != nil {
					t.Fatalf("Failed to decode event %s", err)
				}
				foundContext = convert[1](foundContext)

				if diff := cmp.Diff(convert[0](context), convert[0](foundContext)); diff != "" {
					t.Fatalf("%s: Context was transcoded lossily (-want +got): %s", testName, diff)
				}
				if diff := cmp.Diff(doc, foundData); diff != "" {
					t.Fatalf("%s: Data was transcoded lossily (-want +got): %s", testName, diff)
				}
			})
		}
	}
}

type Address struct {
	City, State string
}
type Person struct {
	XMLName   xml.Name `xml:"person"`
	Id        int      `xml:"id,attr"`
	FirstName string   `xml:"name>first"`
	LastName  string   `xml:"name>last"`
	Age       int      `xml:"age"`
	Height    float32  `xml:"height,omitempty"`
	Married   bool
	Address
	Comment string `xml:",comment"`
}

func (Person) MarshalJSON() ([]byte, error) {
	return nil, errors.New("Person cannot be JSON encoded")
}

func (*Person) UnmarshalJSON([]byte) error {
	return errors.New("Person cannot be JSON decoded")
}

func TestXmlStructuredDecoding(t *testing.T) {
	person := &Person{
		XMLName: xml.Name{
			Local: "person",
		},
		Id:        13,
		FirstName: "John",
		LastName:  "Doe",
		Age:       42,
		Comment:   " Need more details. ",
		Address:   Address{"Hanga Roa", "Easter Island"},
	}

	xmlPerson := `
		<person id="13">
			<name>
				<first>John</first>
				<last>Doe</last>
			</name>
			<age>42</age>
			<Married>false</Married>
			<City>Hanga Roa</City>
			<State>Easter Island</State>
			<!-- Need more details. -->
		</person>`
	xmlJsonSafe, err := json.Marshal(xmlPerson)
	if err != nil {
		t.Fatalf("Failed to create JSON encoded XML string: %s", err)
	}

	eventText := `
	{
		"cloudEventsVersion": "0.1",
		"eventID": "1234",
		"eventType": "dev.eventing.test",
		"source": "tests://TextXmlStructuredEncoding",
		"contentType": "application/xml",
		"data": ` + string(xmlJsonSafe) + `
	}`

	h := http.Header{}
	h.Set(cloudevents.HeaderContentType, cloudevents.ContentTypeStructuredJSON)
	req := &http.Request{
		Header: h,
		Body:   ioutil.NopCloser(strings.NewReader(eventText)),
	}

	var foundPerson Person
	_, err = cloudevents.FromRequest(&foundPerson, req)
	if err != nil {
		t.Fatalf("Failed to parse cross-encoded request: %s", err)
	}

	if diff := cmp.Diff(person, &foundPerson); diff != "" {
		t.Fatalf("Failed to parse xml-encoded data (-want +got): %s", diff)
	}
}

func TestExtensionsAreNeverNil(t *testing.T) {
	r := &http.Request{
		Header: http.Header{
			cloudevents.HeaderContentType: []string{cloudevents.ContentTypeStructuredJSON},
		},
		Body: ioutil.NopCloser(strings.NewReader(`
			{
				"cloudEventsVersion": "0.1",
				"eventID": "1234",
				"eventType": "dev.eventing.test",
				"source": "tests://TextXmlStructuredEncoding",
				"data": "hello, world"
			}`)),
	}

	var data interface{}
	ctx, err := cloudevents.FromRequest(&data, r)
	if err != nil {
		t.Fatal("Failed to parse request", err)
	}
	if ctx.AsV01().Extensions == nil {
		t.Fatal("v0.1 Extensions should never be nil")
	}
	if ctx.AsV02().Extensions == nil {
		t.Fatal("v0.2 Extensions should never be nil")
	}
}

func TestExtensionExtraction(t *testing.T) {
	h := http.Header{}
	h.Set(cloudevents.HeaderCloudEventsVersion, cloudevents.V01CloudEventsVersion)
	h.Set(cloudevents.HeaderContentType, cloudevents.ContentTypeBinaryJSON)
	h.Set(cloudevents.HeaderEventID, "1234")
	h.Set(cloudevents.HeaderEventType, "dev.eventing.test")
	h.Set(cloudevents.HeaderSource, "tests://TestExtensionExtraction")
	h.Set("CE-X-Prop1", "value1")
	h.Set("CE-X-Prop2", `{"nestedProp":"nestedValue"}`)
	b := strings.NewReader(`{"hello": "world"}`)

	r := &http.Request{
		Header: h,
		Body:   ioutil.NopCloser(b),
	}
	var data interface{}
	ctx, err := cloudevents.FromRequest(&data, r)
	if err != nil {
		t.Fatal("Failed to parse request", err)
	}

	expectedV01Extensions := map[string]interface{}{
		"Prop1": "value1",
		"Prop2": map[string]interface{}{
			"nestedProp": "nestedValue",
		},
	}
	if diff := cmp.Diff(expectedV01Extensions, ctx.AsV01().Extensions); diff != "" {
		t.Fatalf("Did not parse expected extensions (-want + got): %s", diff)
	}
}
