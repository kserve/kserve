package http

import (
	"context"
	"fmt"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/transport"
)

// Codec is the wrapper for all versions of codecs supported by the http
// transport.
type Codec struct {
	// Encoding is the setting to inform the DefaultEncodingSelectionFn for
	// selecting a codec.
	Encoding Encoding

	// DefaultEncodingSelectionFn allows for encoding selection strategies to be injected.
	DefaultEncodingSelectionFn EncodingSelector

	v01 *CodecV01
	v02 *CodecV02
	v03 *CodecV03
}

// Adheres to Codec
var _ transport.Codec = (*Codec)(nil)

// Encode encodes the provided event into a transport message.
func (c *Codec) Encode(ctx context.Context, e cloudevents.Event) (transport.Message, error) {
	encoding := c.Encoding

	if encoding == Default && c.DefaultEncodingSelectionFn != nil {
		encoding = c.DefaultEncodingSelectionFn(ctx, e)
	}

	switch encoding {
	case Default:
		fallthrough
	case BinaryV01:
		fallthrough
	case StructuredV01:
		if c.v01 == nil {
			c.v01 = &CodecV01{Encoding: encoding}
		}
		return c.v01.Encode(ctx, e)
	case BinaryV02:
		fallthrough
	case StructuredV02:
		if c.v02 == nil {
			c.v02 = &CodecV02{Encoding: encoding}
		}
		return c.v02.Encode(ctx, e)
	case BinaryV03:
		fallthrough
	case StructuredV03:
		if c.v03 == nil {
			c.v03 = &CodecV03{Encoding: encoding}
		}
		return c.v03.Encode(ctx, e)
	default:
		return nil, fmt.Errorf("unknown encoding: %s", encoding)
	}
}

// Decode converts a provided transport message into an Event, or error.
func (c *Codec) Decode(ctx context.Context, msg transport.Message) (*cloudevents.Event, error) {
	switch c.inspectEncoding(ctx, msg) {
	case BinaryV01, StructuredV01:
		if c.v01 == nil {
			c.v01 = &CodecV01{Encoding: c.Encoding}
		}
		if event, err := c.v01.Decode(ctx, msg); err != nil {
			return nil, err
		} else {
			return c.convertEvent(event), nil
		}

	case BinaryV02, StructuredV02:
		if c.v02 == nil {
			c.v02 = &CodecV02{Encoding: c.Encoding}
		}
		if event, err := c.v02.Decode(ctx, msg); err != nil {
			return nil, err
		} else {
			return c.convertEvent(event), nil
		}

	case BinaryV03, StructuredV03, BatchedV03:
		if c.v03 == nil {
			c.v03 = &CodecV03{Encoding: c.Encoding}
		}
		if event, err := c.v03.Decode(ctx, msg); err != nil {
			return nil, err
		} else {
			return c.convertEvent(event), nil
		}
	default:
		return nil, transport.NewErrMessageEncodingUnknown("wrapper", TransportName)
	}
}

// Give the context back as the user expects
func (c *Codec) convertEvent(event *cloudevents.Event) *cloudevents.Event {
	if event == nil {
		return nil
	}
	switch c.Encoding {
	case Default:
		return event
	case BinaryV01:
		fallthrough
	case StructuredV01:
		if c.v01 == nil {
			c.v01 = &CodecV01{Encoding: c.Encoding}
		}
		ca := event.Context.AsV01()
		event.Context = ca
		return event
	case BinaryV02:
		fallthrough
	case StructuredV02:
		if c.v02 == nil {
			c.v02 = &CodecV02{Encoding: c.Encoding}
		}
		ca := event.Context.AsV02()
		event.Context = ca
		return event
	case BinaryV03:
		fallthrough
	case StructuredV03:
		fallthrough
	case BatchedV03:
		if c.v03 == nil {
			c.v03 = &CodecV03{Encoding: c.Encoding}
		}
		ca := event.Context.AsV03()
		event.Context = ca
		return event
	default:
		return nil
	}
}

func (c *Codec) inspectEncoding(ctx context.Context, msg transport.Message) Encoding {
	// TODO: there should be a better way to make the version codecs on demand.
	if c.v01 == nil {
		c.v01 = &CodecV01{Encoding: c.Encoding}
	}
	// Try v0.1 first.
	encoding := c.v01.inspectEncoding(ctx, msg)
	if encoding != Unknown {
		return encoding
	}

	if c.v02 == nil {
		c.v02 = &CodecV02{Encoding: c.Encoding}
	}
	// Try v0.2.
	encoding = c.v02.inspectEncoding(ctx, msg)
	if encoding != Unknown {
		return encoding
	}

	if c.v03 == nil {
		c.v03 = &CodecV03{Encoding: c.Encoding}
	}
	// Try v0.3.
	encoding = c.v03.inspectEncoding(ctx, msg)
	if encoding != Unknown {
		return encoding
	}

	// We do not understand the message encoding.
	return Unknown
}
