package constant

const (
	ACCEPTABLE_TIME_DIFF = 60
	// https://datatracker.ietf.org/doc/html/rfc5322#section-3.4.1
	// https://stackoverflow.com/a/201378
	USERNAME_VALIDATION_REGEX = "^(?:[a-zA-Z0-9!#$%&'*+\\/=?^_`{|}~-]+(?:\\.[a-z0-9!#$%&'*+\\/=?^_`{|}~-]+)*|\"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*\")$"
	// https://www.rfc-editor.org/errata/eid1690
	MAX_USERNAME_LENGTH = 64

	NWC_MAX_RELAYS_LENGTH = 10
)
