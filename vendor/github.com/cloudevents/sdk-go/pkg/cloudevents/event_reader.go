package cloudevents

import (
	"time"
)

var _ EventReader = (*Event)(nil)

// SpecVersion implements EventReader.SpecVersion
func (e Event) SpecVersion() string {
	if e.Context != nil {
		return e.Context.GetSpecVersion()
	}
	return ""
}

// Type implements EventReader.Type
func (e Event) Type() string {
	if e.Context != nil {
		return e.Context.GetType()
	}
	return ""
}

// Source implements EventReader.Source
func (e Event) Source() string {
	if e.Context != nil {
		return e.Context.GetSource()
	}
	return ""
}

// Subject implements EventReader.Subject
func (e Event) Subject() string {
	if e.Context != nil {
		return e.Context.GetSubject()
	}
	return ""
}

// ID implements EventReader.ID
func (e Event) ID() string {
	if e.Context != nil {
		return e.Context.GetID()
	}
	return ""
}

// Time implements EventReader.Time
func (e Event) Time() time.Time {
	if e.Context != nil {
		return e.Context.GetTime()
	}
	return time.Time{}
}

// SchemaURL implements EventReader.SchemaURL
func (e Event) SchemaURL() string {
	if e.Context != nil {
		return e.Context.GetSchemaURL()
	}
	return ""
}

// DataContentType implements EventReader.DataContentType
func (e Event) DataContentType() string {
	if e.Context != nil {
		return e.Context.GetDataContentType()
	}
	return ""
}

// DataMediaType returns the parsed DataMediaType of the event. If parsing
// fails, the empty string is returned. To retrieve the parsing error, use
// `Context.GetDataMediaType` instead.
func (e Event) DataMediaType() string {
	if e.Context != nil {
		mediaType, _ := e.Context.GetDataMediaType()
		return mediaType
	}
	return ""
}

// DataContentEncoding implements EventReader.DataContentEncoding
func (e Event) DataContentEncoding() string {
	if e.Context != nil {
		return e.Context.GetDataContentEncoding()
	}
	return ""
}

// DataContentEncoding implements EventReader.DataContentEncoding
func (e Event) Extensions() map[string]interface{} {
	if e.Context != nil {
		return e.Context.GetExtensions()
	}
	return map[string]interface{}(nil)
}
