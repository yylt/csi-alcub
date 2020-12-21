package utils

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
)

var (
	AuthHead = "Authorization"
)

func LookupAddresses(fn func(name string, ip net.IP, ipmask net.IPMask) bool) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return err
		}
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPAddr:
				ok := fn(i.Name, v.IP, v.IP.DefaultMask())
				if !ok {
					return nil
				}
			case *net.IPNet:
				ok := fn(i.Name, v.IP, v.Mask)
				if !ok {
					return nil
				}
			}
		}
	}
	return nil
}

func BuildBasicAuthMd5(user, pass []byte) http.Header {
	if len(user) == 0 && len(pass) == 0 {
		return http.Header{}
	}
	//TODO use hex encoder is right?
	bearstr := fmt.Sprintf("%s:%x", user, md5.Sum(pass))
	b64 := base64.StdEncoding.EncodeToString([]byte(bearstr))
	return http.Header{
		AuthHead: []string{fmt.Sprintf("Basic %s", b64)},
	}
}
