package openapi3

import (
	"fmt"
	"regexp"
)

var SchemaStringFormats = make(map[string]*regexp.Regexp, 8)

func DefineStringFormat(name string, pattern string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		err := fmt.Errorf("Format '%v' has invalid pattern '%v': %v", name, pattern, err)
		panic(err)
	}
	SchemaStringFormats[name] = re
}

func init() {
	// This pattern catches only some suspiciously wrong-looking email addresses.
	// Use DefineStringFormat(...) if you need something stricter.
	DefineStringFormat("email", `^[^@]+@[^@<>",\s]+$`)

	// Base64
	// The pattern supports base64 and b./ase64url. Padding ('=') is supported.
	DefineStringFormat("byte", `(^$|^[a-zA-Z0-9+/\-_]*=*$)`)

	// date
	DefineStringFormat("date", `^[0-9]{4}-(0[0-9]|10|11|12)-([0-2][0-9]|30|31)$`)

	// date-time
	DefineStringFormat("date-time", `^[0-9]{4}-(0[0-9]|10|11|12)-([0-2][0-9]|30|31)T[0-9]{2}:[0-9]{2}:[0-9]{2}(.[0-9]+)?(Z|(\+|-)[0-9]{2}:[0-9]{2})?$`)
}
