package dns

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/miekg/dns"
)

type DnsService interface {
	Set(username, offer string) (uint32, error)
	Remove(username string) error
}

func NewNoDns() DnsService {
	return &NoDns{}
}

type NoDns struct{}

func (n *NoDns) Set(username, offer string) (uint32, error) {
	// No DNS implementation, do nothing
	log.Printf("No DNS implementation, not setting username: %s, offer: %s", username, offer)
	return 0, nil
}

func (n *NoDns) Remove(username string) error {
	// No DNS implementation, do nothing
	log.Printf("No DNS implementation, not removing username: %s", username)
	return nil
}

func NewDns(externalURL *url.URL, nameServer, protocol, tsigKey, tsigSecret string) *Dns {
	dnsTimeout := 60 * time.Second
	client := &dns.Client{
		Timeout: dnsTimeout,
		Net:     protocol,
	}
	return &Dns{
		domain:     externalURL.Host,
		nameServer: nameServer,
		tsigKey:    tsigKey,
		tsigSecret: tsigSecret,
		client:     client,
	}
}

type Dns struct {
	domain     string
	nameServer string
	tsigKey    string
	tsigSecret string
	client     *dns.Client
}

func (d *Dns) Set(username, offer string) (uint32, error) {
	ttl := uint32(3600)
	zone := fmt.Sprintf("_bitcoin-payment.%s.", d.domain)
	name := fmt.Sprintf("%s.user.%s", username, zone)
	txt := fmt.Sprintf("bitcoin:?lno=%s", offer)

	rr := new(dns.TXT)
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl}
	rr.Txt = []string{txt}
	rrs := []dns.RR{rr}

	m := new(dns.Msg)
	m.SetUpdate(zone)
	m.Insert(rrs)

	z := dns.Fqdn(d.tsigKey)
	m.SetTsig(z, dns.HmacSHA256, 300, time.Now().Unix())
	d.client.TsigSecret = map[string]string{z: d.tsigSecret}
	reply, _, err := d.client.Exchange(m, d.nameServer)
	if err != nil {
		log.Printf("DNS update failed: %v", err)
		return 0, err
	}
	if reply != nil && reply.Rcode != dns.RcodeSuccess {
		err := fmt.Errorf("server replied: %s", dns.RcodeToString[reply.Rcode])
		log.Printf("DNS update failed: %v", err)
		return 0, err
	}

	return ttl, nil
}

func (d *Dns) Remove(username string) error {
	zone := fmt.Sprintf("_bitcoin-payment.%s.", d.domain)
	name := fmt.Sprintf("%s.user.%s", username, zone)

	rr := new(dns.TXT)
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET}
	rrs := []dns.RR{rr}

	m := new(dns.Msg)
	m.SetUpdate(zone)
	m.RemoveName(rrs)

	z := dns.Fqdn(d.tsigKey)
	m.SetTsig(z, dns.HmacSHA256, 300, time.Now().Unix())
	d.client.TsigSecret = map[string]string{z: d.tsigSecret}
	reply, _, err := d.client.Exchange(m, d.nameServer)
	if err != nil {
		log.Printf("DNS update failed: %v", err)
		return err
	}
	if reply != nil && reply.Rcode != dns.RcodeSuccess {
		err := fmt.Errorf("server replied: %s", dns.RcodeToString[reply.Rcode])
		log.Printf("DNS update failed: %v", err)
		return err
	}

	return nil
}
