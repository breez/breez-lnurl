package persist

import "fmt"

type ErrorUsernameConflict struct {
	username string
	err      error
}

func NewErrorUsernameConflict(username string, err error) error {
	return &ErrorUsernameConflict{username, err}
}

func (e ErrorUsernameConflict) Error() string {
	return fmt.Sprintf("username conflict: %s", e.username)
}
