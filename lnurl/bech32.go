package lnurl

import "github.com/btcsuite/btcd/btcutil/bech32"

func encodeLnurl(s string) (string, error) {
	converted, err := bech32.ConvertBits([]byte(s), 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode("lnurl", converted)
}
