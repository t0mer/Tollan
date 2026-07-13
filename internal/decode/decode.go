// Package decode turns raw journaled bytes into a canonical schema.Message. Each
// input protocol has a decoder; decoding runs in the processing workers so that
// inputs stay thin. Decoders populate Source, Timestamp, Body and Fields; the
// processor assigns the message ID, InputID and stream.
package decode

import (
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

// Decode parses payload according to inputType into a message. Unknown types
// fall back to raw text so no data is lost.
func Decode(inputType, source string, received time.Time, payload []byte) (*schema.Message, error) {
	switch inputType {
	case "syslog":
		return decodeSyslog(source, received, payload)
	case "gelf":
		return decodeGELF(source, received, payload)
	case "cef":
		return decodeCEF(source, received, payload)
	case "httpjson", "beats", "netflow", "ipfix":
		return decodeJSON(source, received, payload)
	case "raw":
		return decodeRaw(source, received, payload), nil
	default:
		return decodeRaw(source, received, payload), nil
	}
}

// decodeRaw treats the payload as a single plain-text line.
func decodeRaw(source string, received time.Time, payload []byte) *schema.Message {
	m := schema.NewMessage(received)
	m.Source = source
	m.Timestamp = received
	m.Body = string(payload)
	return m
}
