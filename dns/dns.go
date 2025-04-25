package dns

import "log"

type Dns interface {
	Set(username, offer string) error
	Remove(username string) error
}

type NoDns struct{}

func (n *NoDns) Set(username, offer string) error {
	// No DNS implementation, do nothing
	log.Printf("No DNS implementation, not setting username: %s, offer: %s", username, offer)
	return nil
}
func (n *NoDns) Remove(username string) error {
	// No DNS implementation, do nothing
	log.Printf("No DNS implementation, not removing username: %s", username)
	return nil
}
